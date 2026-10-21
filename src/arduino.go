package main

import (
	"encoding/json"
	"errors"
	"os/exec"
	"time"
)

type Arduino struct {
	controller       string
	programmer       string
	serialID         string
	portName         string
	bootloaderID     int
	manualPort       bool
	profileID        int
	manuallyAssigned bool
	ardOS            ArduinoOS // структура с данными для поиска устройства на определённой ОС
}

func NewArduinoFromTemp(temp BoardTemplate, portName string, ardOS ArduinoOS, serialID string) *Arduino {
	var arduinoPayload ArduinoPayload
	err := json.Unmarshal(temp.TypePayload, &arduinoPayload)
	if err != nil {
		printLog("Error, wrong arduino payload!")
		return &Arduino{
			controller:   "Unknown",
			programmer:   "Unknown",
			bootloaderID: -1,
			serialID:     serialID,
			portName:     portName,
			ardOS:        ardOS,
			profileID:    temp.ID,
		}
	}
	return &Arduino{
		controller:   arduinoPayload.Controller,
		programmer:   arduinoPayload.Programmer,
		bootloaderID: arduinoPayload.BootloaderID,
		serialID:     serialID,
		portName:     portName,
		ardOS:        ardOS,
		profileID:    temp.ID,
	}
}

func CopyArduino(board *Arduino) *Arduino {
	return &Arduino{
		controller:       board.controller,
		programmer:       board.programmer,
		bootloaderID:     board.bootloaderID,
		serialID:         board.serialID,
		portName:         board.portName,
		ardOS:            board.ardOS,
		manualPort:       board.manualPort,
		profileID:        board.profileID,
		manuallyAssigned: board.manuallyAssigned,
	}
}

func (board *Arduino) avrdude(args ...string) ([]byte, error) {
	defaultArgs := []string{"-D", "-p", board.controller, "-c", board.programmer, "-P", board.portName}
	if configPath != "" {
		defaultArgs = append(defaultArgs, "-C", configPath)
	}
	defaultArgs = append(defaultArgs, args...)
	cmd := exec.Command(avrdudePath, defaultArgs...)
	return cmd.CombinedOutput()
}

// подключено ли устройство
func (board *Arduino) IsConnected() bool {
	return board.portName != NOT_FOUND
}

func (board *Arduino) GetSerialPort() string {
	return board.portName
}

func (board *Arduino) hasBootloader() bool {
	return board.bootloaderID != -1
}

func (board *Arduino) flashBootloader(filePath string, logger chan any) (string, error) {
	flasherSync.Lock()
	defer flasherSync.Unlock()
	if e := rebootPort(board.portName); e != nil {
		return "Не удалось перезагрузить порт", e
	}
	bootloaderType := board.bootloaderID
	detector.DontAddThisType(bootloaderType)
	defer detector.AddThisType(bootloaderType)
	defer time.Sleep(500 * time.Millisecond)
	var notAddedDevices map[string]*Device
	found := false
	for i := 0; i < 25; i++ {
		// TODO: возможно стоит добавить количество необходимого времени в параметры сервера
		time.Sleep(500 * time.Millisecond)
		printLog("Попытка найти подходящее устройство", i+1)
		_, notAddedDevices, _ = detector.Update()
		sameTypeCnt := 0
		var bootloaderDevice *Device
		for _, dev := range notAddedDevices {
			if dev.TypeDesc.ID == bootloaderType {
				bootloaderDevice = dev
				sameTypeCnt++
				if sameTypeCnt > 1 {
					return "Не удалось опознать Bootloader. Ошибка могла быть вызвана перезагрузкой одного из устройств, либо из-за подключения нового.", errors.New("bootloader: too many")
				}
				found = true
			}
		}
		if found {
			return bootloaderDevice.Board.Flash(filePath, logger)
		}
	}
	return "Не удалось найти Bootloader.", errors.New("bootloader: not found")
}

func (board *Arduino) Flash(filePath string, logger chan any) (string, error) {
	if board.hasBootloader() {
		return board.flashBootloader(filePath, logger)
	}
	flashFile := "flash:w:" + getAbolutePath(filePath) + ":a"
	stdout, err := board.avrdude("-U", flashFile)
	avrdudeMessage := handleFlashResult(string(stdout), err)
	return avrdudeMessage, err
}

func (board *Arduino) hasSerial() bool {
	return board.serialID != NOT_FOUND
}

func (board *Arduino) GetWebMessageType() string {
	return DeviceMsg
}

func (board *Arduino) GetWebMessage(name string, deviceID string) any {
	return DeviceMessage{
		ID:               deviceID,
		Name:             name,
		Controller:       board.controller,
		Programmer:       board.programmer,
		SerialID:         board.serialID,
		PortName:         board.portName,
		ProfileID:        board.profileID,
		ManuallyAssigned: board.manuallyAssigned,
	}
}

func (board *Arduino) Ping() error {
	_, err := board.avrdude("-n")
	return err
}

func (board *Arduino) Reset() error {
	_, err := board.avrdude("-r")
	return err
}

func (board *Arduino) GetMetaData() (any, error) {
	return "", errors.New("операция получения метаданных недоступна для этого устройства")
}
