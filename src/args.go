package main

import (
	"flag"
	"fmt"
	"log"
	"time"
)

// адрес на котором будет работать этот сервер
var webAddress string

// максмальный размер одного сообщения, передаваемого через веб-сокеты (в байтах)
var maxMsgSize int

// максимальный размер файла, загружаемого на сервер (в байтах)
var maxFileSize int

// максимальное количество потоков (горутин) на обработку запросов на одного клиента
var maxThreadsPerClient int

/*
минимальное время, через которое клиент может снова запросить список устройств;

игнорируется, если количество клиентов меньше чем 2
*/
var getListCooldownDuration time.Duration

// количество времени между автоматическими обновлениями
var updateListTime time.Duration

// время, в течение которого отсутствующее устройство остаётся в списке
var deviceDisconnectGrace time.Duration

// выводить в консоль подробную информацию
var verbose bool

// всегда искать устройства и обновлять их список, даже когда ни один клиент не подключён (используется для тестирования)
var alwaysUpdate bool

// количество ненастоящих, симулируемых устройств, которые будут восприниматься как настоящие, применяется для тестирования
var fakeBoardsNum int

// количество ненастоящих МС-ТЮК, которые симулируют работу, применяется для тестироваения
var fakeMSNum int

// путь к avrdude
var avrdudePath string

// путь к файлу конфигурации (если пустой, то он не будет передаваться через аргументы в avrdude)
var configPath string

// путь к файлу со списком устройств (будет использован вместо стандартного списка)
var deviceListPath string

// путь к файлу с настройками сервера (если пустой, используется встроенный settings.json)
var settingsPath string

// путь к программе для прошивки кибермишки
var blgMbUploaderPath string

// TODO: генерация аргументов коммандной строки из json файла
/*
чтение флагов и происвоение им стандартных значений.

Значения, указанные здесь, являются запасными: если параметр не указан ни в
командной строке, ни в settings.json, используется значение из регистрации флага.
Приоритет параметров: командная строка > settings.json (settingsPath) > встроенный settings.json.
*/
func setArgs() {
	flag.StringVar(&webAddress, "address", "localhost:8080", "адресс для подключения")
	flag.StringVar(&avrdudePath, "avrdudePath", "avrdude", "путь к avrdude, используется системный путь по-умолчанию")
	flag.StringVar(&configPath, "configPath", "", "путь к файлу конфигурации avrdude")
	flag.StringVar(&deviceListPath, "deviceListPath", "", "путь к JSON-файлу со списком устройств. Если прописан, то заменяет стандартный список устройств, при условии, что не возникнет ошибок, связанных с чтением и открытием JSON-файла, иначе используется стандартный список устройств (по-умолчанию пустая строка, означающая, что будет используется, встроенный в загрузчик список)")
	flag.StringVar(&settingsPath, "settingsPath", "", "путь к JSON-файлу с настройками сервера. Если прописан, то заменяет встроенный settings.json, при условии, что не возникнет ошибок, связанных с чтением и открытием JSON-файла, иначе используются встроенные настройки (по-умолчанию пустая строка, означающая, что будет использован встроенный в загрузчик settings.json). Параметры, явно указанные в командной строке, имеют приоритет над значениями из файла")
	flag.StringVar(&blgMbUploaderPath, "blgMbUploaderPath", "blg-mb/cyberbear-loader", "путь к программе для прошивки кибермишки")
	flag.IntVar(&maxMsgSize, "msgSize", 1024, "максмальный размер одного сообщения, передаваемого через веб-сокеты (в байтах)")
	flag.IntVar(&maxFileSize, "fileSize", 2*1024*1024, "максимальный размер файла, загружаемого на сервер (в байтах)")
	flag.IntVar(&maxThreadsPerClient, "thread", 3, "максимальное количество потоков (горутин) на обработку запросов на одного клиента")
	flag.IntVar(&fakeBoardsNum, "stub", 0, "количество ненастоящих, симулируемых устройств, которые будут восприниматься как настоящие, применяется для тестирования, при значении 0 или меньше фальшивые устройства не добавляются")
	flag.IntVar(&fakeMSNum, "stubms", 0, "количество ненастоящих, симулируемых устройств типа МС-ТЮК, которые будут восприниматься как настоящие, применяется для тестирования, при значении 0 или меньше фальшивые устройства не добавляются")
	flag.BoolVar(&verbose, "verbose", false, "выводить в консоль подробную информацию")
	flag.BoolVar(&alwaysUpdate, "alwaysUpdate", false, "всегда искать устройства и обновлять их список, даже когда ни один клиент не подключён (используется для тестирования)")
	getListCooldownSeconds := flag.Int("listCooldown", 2, "минимальное время (в секундах), через которое клиент может снова запросить список устройств, игнорируется, если количество клиентов меньше чем 2")
	updateListTimeSeconds := flag.Int("updateList", 15, "количество секунд между автоматическими обновлениями, не может быть меньше единицы, если получено значение меньше единицы, то оно заменяется на 1")
	disconnectGraceSeconds := flag.Int("deviceDisconnectGrace", 1, "время (в секундах), в течение которого временно исчезнувшее USB-устройство не удаляется из списка; 0 отключает задержку")
	flag.Parse()

	// параметры, явно переданные через командную строку, имеют приоритет над settings.json
	setFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})
	settings, err := loadSettings(settingsPath)
	if err != nil {
		// ошибка чтения или разбора настроек не останавливает сервер: используются
		// значения по-умолчанию и параметры командной строки
		log.Printf("Не удалось загрузить настройки, используются значения по-умолчанию и параметры командной строки: %v", err)
	}
	applySettings(settings, setFlags, getListCooldownSeconds, updateListTimeSeconds, disconnectGraceSeconds)

	if fakeBoardsNum < 0 {
		fakeBoardsNum = 0
	}
	if fakeMSNum < 0 {
		fakeMSNum = 0
	}
	if *updateListTimeSeconds < 1 {
		*updateListTimeSeconds = 1
	}
	if *disconnectGraceSeconds < 0 {
		*disconnectGraceSeconds = 0
	}
	getListCooldownDuration = time.Second * time.Duration(*getListCooldownSeconds)
	updateListTime = time.Second * time.Duration(*updateListTimeSeconds)
	deviceDisconnectGrace = time.Second * time.Duration(*disconnectGraceSeconds)
}

// вывод описания всех параметров с их значениями
func printArgsDesc() {
	webAddressStr := fmt.Sprintf("адрес: %s", webAddress)
	maxFileSizeStr := fmt.Sprintf("максимальный размер файла: %d", maxFileSize)
	maxMsgSizeStr := fmt.Sprintf("максимальный размер сообщения: %d", maxMsgSize)
	maxThreadsPerClientStr := fmt.Sprintf("максимальное количество потоков (горутин) для обработки запросов на одного клиента: %d", maxThreadsPerClient)
	getListCooldownDurationStr := fmt.Sprintf("перерыв для запроса списка устройств: %v", getListCooldownDuration)
	updateListTimeStr := fmt.Sprintf("промежуток времени между автоматическими обновлениями: %v", updateListTime)
	disconnectGraceStr := fmt.Sprintf("задержка удаления временно отключившегося устройства: %v", deviceDisconnectGrace)
	verboseStr := fmt.Sprintf("вывод подробной информации в консоль: %v", verbose)
	alwaysUpdateStr := fmt.Sprintf("постоянное обновление списка устройств: %v", alwaysUpdate)
	fakeBoardsNumStr := fmt.Sprintf("количество фальшивых устройств: %d", fakeBoardsNum)
	fakeMSNumStr := fmt.Sprintf("количество фальшивых МС-ТЮК: %d", fakeMSNum)
	avrdudePathStr := fmt.Sprintf("путь к avrdude (если написано avrdude, то используется системный путь): %s", avrdudePath)
	configPathStr := fmt.Sprintf("путь к файлу конфигурации avrdude: %s", configPath)
	deviceListPathStr := fmt.Sprintf("путь к файлу со списком устройств (если пусто, то используется встроенный список): %s", deviceListPath)
	blgMbUploaderPathStr := fmt.Sprintf("путь к программе для прошивки кибермишки: %s", blgMbUploaderPath)
	settingsPathStr := fmt.Sprintf("путь к файлу с настройками сервера (если пусто, то используется встроенный settings.json): %s", settingsPath)
	log.Printf("Модуль загрузчика запущен со следующими параметрами:\n %s\n %s\n %s\n %s\n %s\n %s\n %s\n %s\n %s\n %s\n %s\n %s\n %s\n %s\n %s\n %s\n",
		webAddressStr,
		maxFileSizeStr,
		maxMsgSizeStr,
		maxThreadsPerClientStr,
		getListCooldownDurationStr,
		updateListTimeStr,
		disconnectGraceStr,
		verboseStr,
		alwaysUpdateStr,
		fakeBoardsNumStr,
		fakeMSNumStr,
		avrdudePathStr,
		configPathStr,
		deviceListPathStr,
		blgMbUploaderPathStr,
		settingsPathStr,
	)
}
