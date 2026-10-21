//go:build windows
// +build windows

package main

import (
	"fmt"
	"log"
	"os/exec"
	"sort"

	"strings"

	"golang.org/x/sys/windows/registry"
)

type ArduinoOS struct {
	pathToDevice string
}

type MS1OS struct {
	pathesToDevices [4]string
}

// настройка ОС (для Windows она не требуется, но она здесь присутствует, чтобы обеспечить совместимость с другими платформами, которые использует свои реализации этой функции)
func setupOS() {

}

func getAllRegistryValues(path string) ([]string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	defer func() {
		if err := key.Close(); err != nil {
			printLog("Error on closing registry key:", err.Error())
		}
	}()
	if err != nil {
		return nil, err
	}
	registryValues, err := key.ReadValueNames(0)
	if err != nil {
		return nil, err
	}
	return registryValues, nil
}
func handleCloseRegistryKey(key registry.Key, path string) {
	if err := key.Close(); err != nil {
		printLog(fmt.Sprintf("Error on closing registry key in %s: %s", path, err.Error()))
	}
}
func getRegistryValues(path string) (registry.Key, []string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		printLog(fmt.Sprintf("Can't open %s registry key. %s", path, err.Error()))
		return key, nil, err
	}
	registryValues, err := key.ReadValueNames(0)
	if err != nil {
		printLog(fmt.Sprintf("Can't read values names in %s. %s", path, err.Error()))
		return key, nil, err
	}
	return key, registryValues, nil
}

/*
Возвращает пути к подключенным устройствам.

Если substring - не пустая строка, то возращает пути к устройствам, которые содержат substring.

Если ничего не удалось найти, то возвращает nil
*/
func getInstanceId(substring string) []string {
	// для сравнения строк в одном регистре
	substringUP := strings.ToUpper(substring)
	// получаем список, подключенных COM-портов
	const SERIALCOMM_PATH = "HARDWARE\\DEVICEMAP\\SERIALCOMM"
	serialCommKey, serialCommRegistryValues, err := getRegistryValues(SERIALCOMM_PATH)
	defer handleCloseRegistryKey(serialCommKey, SERIALCOMM_PATH)
	if err != nil {
		return nil
	}
	numOfActivePorts := len(serialCommRegistryValues)
	var currentPorts = make([]string, numOfActivePorts)
	for i, valueName := range serialCommRegistryValues {
		value, _, err := serialCommKey.GetStringValue(valueName)
		if err != nil {
			if err == registry.ErrUnexpectedType {
				continue
			}
			printLog(fmt.Sprintf("Error on getting registry values in %s. %s", SERIALCOMM_PATH, err.Error()))
			continue
		}
		currentPorts[i] = value
	}

	// получаем пути к устройствам, соотносим их со списком активным COM портов
	const DEVICE_PATHES = "SYSTEM\\CurrentControlSet\\Control\\COM Name Arbiter\\Devices"
	deviceKey, deviceRegistryValues, err := getRegistryValues(DEVICE_PATHES)
	defer handleCloseRegistryKey(deviceKey, DEVICE_PATHES)
	if err != nil {
		return nil
	}
	deviceRegistryValuesLen := len(deviceRegistryValues)
	sort.Strings(currentPorts)
	sort.Strings(deviceRegistryValues)
	currentPortsIndex := 0
	deviceRegistryValuesIndex := 0
	var pathesToDevices []string
	for currentPortsIndex < numOfActivePorts && deviceRegistryValuesIndex < deviceRegistryValuesLen {
		if currentPorts[currentPortsIndex] < deviceRegistryValues[deviceRegistryValuesIndex] {
			currentPortsIndex++
			continue
		}
		if currentPorts[currentPortsIndex] > deviceRegistryValues[deviceRegistryValuesIndex] {
			deviceRegistryValuesIndex++
			continue
		}
		if currentPorts[currentPortsIndex] == deviceRegistryValues[deviceRegistryValuesIndex] {
			pathToDevice, _, err := deviceKey.GetStringValue(currentPorts[currentPortsIndex])
			currentPortsIndex++
			deviceRegistryValuesIndex++
			if err != nil {
				if err == registry.ErrUnexpectedType {
					continue
				}
				printLog(fmt.Sprintf("Error on getting registry values in %s. %s", DEVICE_PATHES, err.Error()))
				continue
			}
			// здесь идёт преобразование переменной в путь к устройству
			pathToDevice = strings.ToUpper(pathToDevice)
			startIndex := strings.Index(pathToDevice, "USB")
			endIndex := strings.Index(pathToDevice, "#{")
			if startIndex < 0 || endIndex < 0 {
				printLog("Error, can't parse path!")
				continue
			}
			pathToDevice = pathToDevice[startIndex:endIndex]
			pathToDevice = strings.ReplaceAll(pathToDevice, "#", "\\")
			if substring == "" || strings.Contains(pathToDevice, substringUP) {
				pathesToDevices = append(pathesToDevices, pathToDevice)
			}
		}
	}
	return pathesToDevices
}

func getDevs(serviceName string) []string {
	SERVICE_ENUM_PATH := fmt.Sprintf("SYSTEM\\CurrentControlSet\\Services\\%s\\enum", serviceName)
	key, values, err := getRegistryValues(SERVICE_ENUM_PATH)
	if err != nil {
		printLog(err.Error())
		return nil
	}
	defer handleCloseRegistryKey(key, SERVICE_ENUM_PATH)
	devs := []string{}
	for _, valueName := range values {
		value, _, err := key.GetStringValue(valueName)
		if err != nil {
			continue
		}
		devs = append(devs, value)
	}
	return devs
}

// находит все подключённые платы
func detectBoards(boardTemplates []BoardTemplate) map[string]*Device {
	//startTime := time.Now()
	devs := make(map[string]*Device)
	presentUSBDevices := append(getInstanceId(""), append(getDevs("libusb0"), getDevs("WINUSB")...)...)
	// нет usb-устройств
	if presentUSBDevices == nil {
		return nil
	}
	// структура для хранения одной части МС-ТЮК, которую удалось найти
	type ms1Part struct {
		template     *BoardTemplate
		portName     string
		pathToDevice string
		// части МС-ТЮК имеют имена, которые сожержат слова SerialA, SerialB, SerialC, SerialD,
		// порядок портов в МС-ТЮК должен соответствовать алфавитному порядку этих слов
		friendlyName string
	}
	// все части МС-ТЮК, которые удалось найти, используется для того, чтобы "собрать" из них полноценные МС-ТЮКи
	var ms1parts []ms1Part
	for _, line := range presentUSBDevices {
		device := strings.TrimSpace(line)
		deviceLen := len(device)
		for _, boardTemplate := range boardTemplates {
			for _, pidvid := range boardTemplate.PidVid {
				vendorID := pidvid.VendorID
				productID := pidvid.ProductID
				pathPattern := fmt.Sprintf("USB\\VID_%s&PID_%s", vendorID, productID)
				pathLen := len(pathPattern)
				// нашли подходящее устройство
				if pathLen <= deviceLen && strings.EqualFold(device[:pathLen], pathPattern) {
					portName := findPortName(&device)
					if portName == NOT_FOUND {
						printLog("WARNING: No port for device:", device)
					}
					if boardTemplate.IsMSDevice() {
						// сбор МС-ТЮК "по-частям"

						// поиск "FriendlyName"

						keyPath := fmt.Sprintf("SYSTEM\\CurrentControlSet\\Enum\\%s", device)
						key, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, registry.QUERY_VALUE)
						if err != nil {
							printLog("can't open key for ms1-device.", err.Error())
							continue
						}
						friendlyName, _, err := key.GetStringValue("FriendlyName")
						if err != nil {
							printLog("can't get FriendlyName property for ms1-device.", err.Error())
							key.Close()
							continue
						}
						key.Close()
						ms1parts = append(ms1parts, ms1Part{
							template:     &boardTemplate,
							portName:     portName,
							pathToDevice: device,
							friendlyName: friendlyName,
						})
					} else {
						// поиск серийного номера
						serialIndex := strings.LastIndex(device, "\\")
						possibleSerialID := device[serialIndex+1:]
						if strings.Contains(possibleSerialID, "&") {
							possibleSerialID = ""
						}
						if boardTemplate.IsArduinoDevice() {
							detectedBoard := NewArduinoFromTemp(boardTemplate, portName, ArduinoOS{pathToDevice: device}, possibleSerialID)
							devs[device] = newDevice(boardTemplate, detectedBoard)
							printLog("Arduino device was found:", detectedBoard, device)
						} else if boardTemplate.IsBlgMbDevice() {
							devs[device] = newDevice(boardTemplate, &BlgMb{serialID: possibleSerialID})
							printLog("CyberBear device was found:", device)
						} else {
							//TODO
							printLog("no searching algorithm for this type of device!", boardTemplate.Type)
						}
					}
				}
			}
		}
	}
	// windows распознает один МС-ТЮК как 4 разных устройства, поэтому их нужно "объединить" в одно устройство для загрузчика
	sort.Slice(ms1parts, func(i, j int) bool {
		port1 := ms1parts[i].portName
		port2 := ms1parts[j].portName
		len1 := len(port1)
		len2 := len(port2)
		if len1 == len2 {
			return port1 < port2
		}
		return len1 < len2
	})
	msPartsNum := len(ms1parts)
	if msPartsNum%4 != 0 {
		log.Println("Incorrect number of ms1 parts! Can't identify them")
		return devs
	}
	for msPart := 0; msPart < msPartsNum; msPart += 4 {
		pack := ms1parts[msPart : msPart+4]
		sort.Slice(pack, func(i, j int) bool {
			return pack[i].friendlyName < pack[j].friendlyName
		})
		var portNames [4]string
		var pathesToDevices [4]string
		for i := range 4 {
			portNames[i] = pack[i].portName
			pathesToDevices[i] = pack[i].pathToDevice
		}
		ms1 := NewMS1(portNames, MS1OS{pathesToDevices: pathesToDevices})
		devs[pack[0].pathToDevice] = newDevice(*pack[0].template, ms1)
	}
	//endTime := time.Now()
	//printLog("Detection time: ", endTime.Sub(startTime))
	printLog(devs)
	return devs
}

/*
возвращает имя порта, либо константу NOT_FOUND, если не удалось его не удалось найти

TODO: переделать интерфейс функции для всех платформ, сделать, чтобы функция возвращала error

TODO: обновление нескольких портов
*/
func findPortName(instanceId *string) string {
	keyPath := fmt.Sprintf("SYSTEM\\CurrentControlSet\\Enum\\%s\\Device Parameters", *instanceId)
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, registry.QUERY_VALUE)
	defer func() {
		if err := key.Close(); err != nil {
			printLog("Error on closing registry key:", err.Error())
		}
	}()
	if err != nil {
		printLog("Registry error:", err)
		return NOT_FOUND
	}
	portName, _, err := key.GetStringValue("PortName")
	if err == registry.ErrNotExist {
		printLog("Port name doesn't exists")
		return NOT_FOUND
	}
	if err != nil {
		printLog("Error on getting port name:", err.Error())
		return NOT_FOUND
	}
	return portName
}

// true - если порт изменился или не найден, иначе false
// назначает порту значение NOT_FOUND, если не удалось найти порт
func updatePortName(pathToDevice string) string {
	instanceId := getInstanceId(pathToDevice)
	// такого устройства нет
	if instanceId == nil {
		return NOT_FOUND
	}
	if len(instanceId) > 1 {
		log.Printf("updatePortName: found more than one devices that are matched ID = %s\n", pathToDevice)
		return NOT_FOUND
	}
	return findPortName(&pathToDevice)
}

// перезагрузка порта
func rebootPort(portName string) (err error) {
	cmd := exec.Command("MODE", portName, "BAUD=1200")
	_, err = cmd.CombinedOutput()
	if err != nil {
		printLog(cmd.Args, err)
	}
	return err
}

func (board *Arduino) Update() bool {
	if board.manualPort {
		return updateManualPort(&board.portName)
	}
	newPortName := updatePortName(board.ardOS.pathToDevice)
	if newPortName != board.portName {
		board.portName = newPortName
		return true
	}
	return false
}

func (board *MS1) Update() bool {
	return false
}
