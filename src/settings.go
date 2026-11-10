package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
)

/*
ServerSettings описывает настройки сервера, которые можно задать в файле settings.json.

Имена полей в JSON совпадают с именами соответствующих параметров командной строки.
Все поля являются указателями, чтобы можно было отличить отсутствующее в файле
значение от нулевого: если параметр не указан в settings.json, то используется
значение по-умолчанию из командной строки.

Длительности (listCooldown, updateList, deviceDisconnectGrace) задаются в секундах.
*/
type ServerSettings struct {
	WebAddress             *string `json:"address"`
	MaxMsgSize             *int    `json:"msgSize"`
	MaxFileSize            *int    `json:"fileSize"`
	MaxThreadsPerClient    *int    `json:"thread"`
	GetListCooldownSeconds *int    `json:"listCooldown"`
	UpdateListSeconds      *int    `json:"updateList"`
	DisconnectGraceSeconds *int    `json:"deviceDisconnectGrace"`
	Verbose                *bool   `json:"verbose"`
	AlwaysUpdate           *bool   `json:"alwaysUpdate"`
	FakeBoardsNum          *int    `json:"stub"`
	FakeMSNum              *int    `json:"stubms"`
	AvrdudePath            *string `json:"avrdudePath"`
	ConfigPath             *string `json:"configPath"`
	DeviceListPath         *string `json:"deviceListPath"`
	BlgMbUploaderPath      *string `json:"blgMbUploaderPath"`
}

// настройки, встроенные в бинарник, используются, если settingsPath не указан
//
//go:embed settings.json
var defaultSettingsRaw []byte

/*
Загрузка настроек сервера.

Если path пуст, используются настройки, встроенные в бинарник (settings.json).
Иначе настройки читаются из файла по указанному пути.
*/
func loadSettings(path string) (*ServerSettings, error) {
	raw := defaultSettingsRaw
	source := "встроенный settings.json"
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("не удалось прочитать файл настроек %q: %w", path, err)
		}
		raw = data
		source = path
	}

	var settings ServerSettings
	decoder := json.NewDecoder(bytes.NewReader(raw))
	// неизвестные параметры считаются ошибкой, чтобы опечатки в settings.json не игнорировались молча
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil {
		return nil, fmt.Errorf("не удалось разобрать настройки из %q: %w", source, err)
	}
	return &settings, nil
}

/*
Применение настроек к параметрам сервера.

Значения из settings.json используются только для тех параметров, которые не были
явно указаны в командной строке: параметры командной строки имеют приоритет.

setFlags содержит имена параметров, переданных через командную строку;
getListCooldownSeconds, updateListSeconds и disconnectGraceSeconds — указатели на
значения соответствующих параметров командной строки, заданных в секундах.
*/
func applySettings(settings *ServerSettings, setFlags map[string]bool,
	getListCooldownSeconds, updateListSeconds, disconnectGraceSeconds *int) {
	if settings == nil {
		return
	}
	applySetting(setFlags["address"], settings.WebAddress, &webAddress)
	applySetting(setFlags["msgSize"], settings.MaxMsgSize, &maxMsgSize)
	applySetting(setFlags["fileSize"], settings.MaxFileSize, &maxFileSize)
	applySetting(setFlags["thread"], settings.MaxThreadsPerClient, &maxThreadsPerClient)
	applySetting(setFlags["listCooldown"], settings.GetListCooldownSeconds, getListCooldownSeconds)
	applySetting(setFlags["updateList"], settings.UpdateListSeconds, updateListSeconds)
	applySetting(setFlags["deviceDisconnectGrace"], settings.DisconnectGraceSeconds, disconnectGraceSeconds)
	applySetting(setFlags["verbose"], settings.Verbose, &verbose)
	applySetting(setFlags["alwaysUpdate"], settings.AlwaysUpdate, &alwaysUpdate)
	applySetting(setFlags["stub"], settings.FakeBoardsNum, &fakeBoardsNum)
	applySetting(setFlags["stubms"], settings.FakeMSNum, &fakeMSNum)
	applySetting(setFlags["avrdudePath"], settings.AvrdudePath, &avrdudePath)
	applySetting(setFlags["configPath"], settings.ConfigPath, &configPath)
	applySetting(setFlags["deviceListPath"], settings.DeviceListPath, &deviceListPath)
	applySetting(setFlags["blgMbUploaderPath"], settings.BlgMbUploaderPath, &blgMbUploaderPath)
}

/*
applySetting записывает значение src в dst, если параметр не был явно указан в
командной строке (explicitlySet == false) и присутствует в settings.json (src != nil).
*/
func applySetting[T any](explicitlySet bool, src, dst *T) {
	if !explicitlySet && src != nil {
		*dst = *src
	}
}
