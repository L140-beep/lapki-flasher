package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestEmbeddedSettingsContainsAllParams проверяет, что встроенный settings.json
// разбирается и содержит все настройки, представленные в args.go.
func TestEmbeddedSettingsContainsAllParams(t *testing.T) {
	settings, err := loadSettings("")
	if err != nil {
		t.Fatalf("loadSettings(\"\") вернул ошибку: %v", err)
	}

	fields := map[string]any{
		"address":               settings.WebAddress,
		"msgSize":               settings.MaxMsgSize,
		"fileSize":              settings.MaxFileSize,
		"thread":                settings.MaxThreadsPerClient,
		"listCooldown":          settings.GetListCooldownSeconds,
		"updateList":            settings.UpdateListSeconds,
		"deviceDisconnectGrace": settings.DisconnectGraceSeconds,
		"verbose":               settings.Verbose,
		"alwaysUpdate":          settings.AlwaysUpdate,
		"stub":                  settings.FakeBoardsNum,
		"stubms":                settings.FakeMSNum,
		"avrdudePath":           settings.AvrdudePath,
		"configPath":            settings.ConfigPath,
		"deviceListPath":        settings.DeviceListPath,
		"blgMbUploaderPath":     settings.BlgMbUploaderPath,
	}
	for name, field := range fields {
		if field == nil || reflect.ValueOf(field).IsNil() {
			t.Errorf("встроенный settings.json не содержит параметр %q", name)
		}
	}
}

// TestLoadSettingsFromFile проверяет загрузку настроек из указанного файла.
func TestLoadSettingsFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	content := `{"address": "0.0.0.0:9090", "thread": 8, "verbose": true}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("не удалось создать файл настроек: %v", err)
	}

	settings, err := loadSettings(path)
	if err != nil {
		t.Fatalf("loadSettings(%q) вернул ошибку: %v", path, err)
	}
	if settings.WebAddress == nil || *settings.WebAddress != "0.0.0.0:9090" {
		t.Errorf("неверный адрес: %v", settings.WebAddress)
	}
	if settings.MaxThreadsPerClient == nil || *settings.MaxThreadsPerClient != 8 {
		t.Errorf("неверное количество потоков: %v", settings.MaxThreadsPerClient)
	}
	if settings.Verbose == nil || !*settings.Verbose {
		t.Errorf("неверный флаг подробного вывода: %v", settings.Verbose)
	}
	// параметр, отсутствующий в файле, остаётся не заданным, чтобы использовать значение по-умолчанию
	if settings.MaxMsgSize != nil {
		t.Errorf("отсутствующий в файле параметр должен остаться nil, получено %v", *settings.MaxMsgSize)
	}
}

// TestLoadSettingsErrors проверяет, что отсутствующий файл и неизвестный параметр
// считаются ошибкой, при которой используются значения по-умолчанию.
func TestLoadSettingsErrors(t *testing.T) {
	if _, err := loadSettings(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("ожидалась ошибка при чтении отсутствующего файла настроек")
	}

	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"unknownParameter": 1}`), 0o600); err != nil {
		t.Fatalf("не удалось создать файл настроек: %v", err)
	}
	if _, err := loadSettings(path); err == nil {
		t.Error("ожидалась ошибка при неизвестном параметре в файле настроек")
	}
}

// TestApplySettingsCommandLinePriority проверяет, что параметры командной строки
// имеют приоритет над settings.json, а остальные параметры берутся из файла.
func TestApplySettingsCommandLinePriority(t *testing.T) {
	oldWebAddress, oldMaxMsgSize := webAddress, maxMsgSize
	defer func() {
		webAddress, maxMsgSize = oldWebAddress, oldMaxMsgSize
	}()

	fileWebAddress, fileMaxMsgSize := "from-file:1", 4096
	fileCooldown, fileUpdateList, fileGrace := 9, 30, 5
	settings := &ServerSettings{
		WebAddress:             &fileWebAddress,
		MaxMsgSize:             &fileMaxMsgSize,
		GetListCooldownSeconds: &fileCooldown,
		UpdateListSeconds:      &fileUpdateList,
		DisconnectGraceSeconds: &fileGrace,
	}
	cooldown, updateList, grace := 2, 15, 1

	// параметры не указаны в командной строке — используются значения из settings.json
	webAddress, maxMsgSize = "default:0", 1024
	applySettings(settings, map[string]bool{}, &cooldown, &updateList, &grace)

	if webAddress != fileWebAddress {
		t.Errorf("адрес: получено %q, ожидалось %q", webAddress, fileWebAddress)
	}
	if maxMsgSize != fileMaxMsgSize {
		t.Errorf("размер сообщения: получено %d, ожидалось %d", maxMsgSize, fileMaxMsgSize)
	}
	if cooldown != fileCooldown || updateList != fileUpdateList || grace != fileGrace {
		t.Errorf("длительности: получено %d/%d/%d, ожидалось %d/%d/%d",
			cooldown, updateList, grace, fileCooldown, fileUpdateList, fileGrace)
	}

	// параметры указаны в командной строке — приоритет у командной строки
	webAddress, maxMsgSize = "cli:2", 555
	applySettings(settings, map[string]bool{"address": true, "msgSize": true}, &cooldown, &updateList, &grace)

	if webAddress != "cli:2" {
		t.Errorf("адрес: получено %q, ожидалось значение из командной строки %q", webAddress, "cli:2")
	}
	if maxMsgSize != 555 {
		t.Errorf("размер сообщения: получено %d, ожидалось значение из командной строки 555", maxMsgSize)
	}
}

// TestApplySettingsNil проверяет, что при ошибке загрузки настроек (settings == nil)
// значения не изменяются.
func TestApplySettingsNil(t *testing.T) {
	oldWebAddress := webAddress
	defer func() { webAddress = oldWebAddress }()

	webAddress = "default:0"
	cooldown, updateList, grace := 2, 15, 1
	applySettings(nil, map[string]bool{}, &cooldown, &updateList, &grace)

	if webAddress != "default:0" {
		t.Errorf("адрес изменился при пустых настройках: %q", webAddress)
	}
}
