package main

import (
	"container/list"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

type Detector struct {
	// список доступных для прошивки устройств
	boards         map[string]*Device
	detectedBoards map[string]*Device
	manualProfiles map[string]int
	boardTemplates []BoardTemplate
	mu             sync.Mutex

	// симуляция плат
	fakeBoards map[string]*Device

	// Список ID типов плат, которые не нужно добавлять, при обновлении.
	// Старые устройства, если они не отсоединялись, останутся в списке, даже если их typeID находится в списке
	dontAddTypes map[int]void

	// Время первого снимка, в котором плата не была обнаружена.
	missingSince map[string]time.Time

	boardActions *list.List
}

func NewDetector() *Detector {
	var d Detector
	d.boards = make(map[string]*Device)
	d.manualProfiles = make(map[string]int)
	// добавление фальшивых плат
	d.generateFakeBoards()
	d.initDeviceListErrorHandle(deviceListPath)
	d.dontAddTypes = make(map[int]void)
	d.missingSince = make(map[string]time.Time)
	d.boardActions = list.New()
	return &d
}

/*
Обновление текущего списка устройств.

Вовращает:

detectedBoards - все платы, которые удалось обнаружить;

notAddedDevices - список новых устройств, которые были обнаружены, но не были добавлены, так как их типы были добавлены в исключения dontAddTypes;

devicesInList - текущий список плат, без учёта notAddedDevices
*/
func (d *Detector) Update() (
	detectedBoards map[string]*Device,
	notAddedDevices map[string]*Device,
	devicesInList map[string]*Device) {

	d.mu.Lock()
	defer d.mu.Unlock()

	detectedBoards = detectAllBoards(d.boardTemplates)
	detectedBoards = d.withBoardProfiles(detectedBoards)

	// добавление фальшивых плат к действительно обнаруженным
	if fakeBoardsNum > 0 || fakeMSNum > 0 {
		if detectedBoards == nil {
			detectedBoards = make(map[string]*Device)
		}
		for ID, board := range d.fakeBoards {
			detectedBoards[ID] = board
		}
	}

	// обновление информации о старых устройствах и добавление новых

	notAddedDevices = make(map[string]*Device)

	for deviceID, newBoard := range detectedBoards {
		// плата вернулась до окончания grace-периода (либо не исчезала).
		delete(d.missingSince, deviceID)
		oldBoard, exists := d.boards[deviceID]
		if exists {
			oldBoard.Mu.Lock()
			switch oldBoard.Board.(type) {
			case *Arduino:
				oldArduino := oldBoard.Board.(*Arduino)
				newArduino := newBoard.Board.(*Arduino)
				if oldArduino.portName != newArduino.portName {
					oldArduino.portName = newArduino.portName
					d.boardActions.PushBack(ActionWithBoard{board: oldBoard, boardID: deviceID, action: PORT_UPDATE})
				}
			case *BlgMb:
				blgBoard := oldBoard.Board.(*BlgMb)
				if blgBoard.version == "" {
					blgBoard.GetVersion()
				}
				if blgBoard.serialID == "" {
					blgBoard.GetId()
				}
			}
			oldBoard.Mu.Unlock()
		} else {
			if _, ok := d.dontAddTypes[newBoard.TypeDesc.ID]; ok {
				notAddedDevices[deviceID] = newBoard
			} else {
				switch newBoard.Board.(type) {
				case *BlgMb:
					blgBoard := newBoard.Board.(*BlgMb)
					blgBoard.GetVersion()
					if blgBoard.serialID == "" {
						blgBoard.GetId()
					}
				}
				d.boards[deviceID] = newBoard
				d.boardActions.PushBack(ActionWithBoard{board: newBoard, boardID: deviceID, action: ADD})
			}
		}
	}

	// удаление
	now := time.Now()
	for deviceID := range d.boards {
		board, exists := detectedBoards[deviceID]
		if !exists {
			missingAt, alreadyMissing := d.missingSince[deviceID]
			if !alreadyMissing {
				d.missingSince[deviceID] = now
				continue
			}
			// краткое исчезновение (например при перезагрузке) не должно удалять устройство
			if now.Sub(missingAt) < deviceDisconnectGrace {
				continue
			}
			d.boardActions.PushBack(ActionWithBoard{board: board, boardID: deviceID, action: DELETE})
			delete(d.boards, deviceID)
			delete(d.manualProfiles, deviceID)
			delete(d.missingSince, deviceID)
		}
	}
	devicesInList = d.boards
	return
}

// Возвращает и удаляет первое действие в очереди.
// Если список действий пуст, то возвращает пустое действие и ложь в качестве второй переменной.
// Иначе вовращает действие с платой и истину.
func (d *Detector) PopFrontActionSync() (action ActionWithBoard, exists bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.boardActions.Len() > 0 {
		return d.boardActions.Remove(d.boardActions.Front()).(ActionWithBoard), true
	} else {
		return ActionWithBoard{}, false
	}
}

// попросить дектектора, чтобы он не добавлял новые устройства с данным typeID в список (старые устройства с этим typeID останутся в списке)
func (d *Detector) DontAddThisType(typeID int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dontAddTypes[typeID] = void{}
}

func (d *Detector) AddThisType(typeID int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.dontAddTypes, typeID)
}

// возвращает устройство, соответствующее ID, существует ли устройство в списке
func (d *Detector) GetBoardSync(ID string) (*Device, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	value, exists := d.boards[ID]
	return value, exists
}

func (d *Detector) AddBoardSync(ID string, board *Device) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.boards[ID] = board
}

// удаляет устройство из списка
func (d *Detector) DeleteBoard(ID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.boards, ID)
	delete(d.manualProfiles, ID)
	delete(d.detectedBoards, ID)
}

// получить количество, подключённых плат
func (d *Detector) boardsNum() int {
	return len(d.boards)
}

// удаляем устройства, которых больше нет и уведомляем об этом всех клиентов
func (d *Detector) DeleteAndAlert(newBoards map[string]*Device, c *WebSocketConnection) {
	d.mu.Lock()
	for deviceID := range detector.boards {
		_, exists := newBoards[deviceID]
		if !exists {
			delete(detector.boards, deviceID)
			DeviceUpdateDelete(deviceID, c)
		}
	}
	d.mu.Unlock()
}

func (d *Detector) boardList() []BoardTemplate {
	return d.boardTemplates
}

// генерация фальшивых плат, которые будут восприниматься программой как настоящие
func (d *Detector) generateFakeBoards() {
	d.fakeBoards = make(map[string]*Device)

	// фальшивые параметры для фальшивых плат
	controller := "Fake Controller"
	programmer := "Fake Programmer"
	fakeArduinoTemp := BoardTemplate{
		ID:                 -1,
		Name:               "Fake Board",
		PidVid:             []PidVidType{},
		Type:               "Fake arduino",
		FlashFileExtension: "hex",
	}
	// генерация фальшивых ардуино-подобных устройств
	for i := 0; i < fakeBoardsNum; i++ {
		fakeID := fmt.Sprintf("fakeid-%d", i)
		fakePort := fmt.Sprintf("fakecom-%d", i)
		newFakeBoard := newDevice(fakeArduinoTemp, &FakeBoard{
			controller: controller,
			programmer: programmer,
			portName:   fakePort,
			serialID:   fakeID,
		})
		d.fakeBoards[fakeID] = newFakeBoard
	}

	fakeMsTemp := BoardTemplate{
		ID:                 -2,
		Name:               "Fake ms1",
		PidVid:             []PidVidType{},
		Type:               "Fake ms",
		FlashFileExtension: "bin",
	}
	// генерация фальшивых МС-ТЮК
	for i := 0; i < fakeMSNum; i++ {
		fakeID := fmt.Sprintf("fakeidms-%d", i)
		var fakePorts [4]string
		for j := 0; j < 4; j++ {
			fakePorts[j] = fmt.Sprintf("fms-%d", i+j)
		}
		var fakeAddress string
		// тут может быть проблема, если количество фальшивых МС-ТЮК огромнно
		for j := 0; j < 16-len(fakeID); j++ {
			fakeAddress += "0"
		}
		fakeAddress += fakeID
		newFakeBoard := newDevice(
			fakeMsTemp,
			&FakeMS{
				portNames:     fakePorts,
				fakeAddress:   fakeAddress,
				clientAddress: "",
			},
		)
		d.fakeBoards[fakeID] = newFakeBoard
	}
	for ID, board := range d.fakeBoards {
		d.boards[ID] = board
	}
}

//go:embed device_list.JSON
var boardTemplatesRaw []byte

/*
Добавление списка устройств в детектор.

pathToList - путь к json-файлу со списком устройств.
Если pathToList - пуст, то используется стандартный список (boardTemplatesRaw)
*/
func (d *Detector) initDeviceList(pathToList string) error {
	if pathToList == "" {
		err := json.Unmarshal(boardTemplatesRaw, &d.boardTemplates)
		if err != nil {
			return err
		}
	} else {
		jsonFile, err := os.Open(pathToList)
		if err != nil {
			//log.Println("Can't open json file with custom device list. Standard device list will be used instead.", err.Error())
			return err
		}
		defer jsonFile.Close()
		byteValue, err := io.ReadAll(jsonFile)
		if err != nil {
			//log.Println("Can't read json file with custom device list. Standard device list will be used instead.", err.Error())
			return err
		}
		err = json.Unmarshal(byteValue, &d.boardTemplates)
		if err != nil {
			//log.Println("Can't unmarshal json file with custom device list. Standard device list will be used instead.", err.Error())
			return err
		}
	}
	return nil
}

/*
Обработка ошибок, связанных с добавлением списка устройств в детектор.

pathToList - путь к json-файлу со списком устройств.
Если pathToList - пуст, то используется стандартный список (boardTemplatesRaw)
*/
func (d *Detector) initDeviceListErrorHandle(pathToList string) {
	err := d.initDeviceList(pathToList)
	if err != nil {
		if pathToList == "" {
			log.Fatal("Can't run lapki-flasher because failed to use standart device list. Device detector won't be able to work!", err.Error())
		} else {
			log.Println("Can't use json file with custom device list because of error. Standard device list will be used instead.", err.Error())
			d.initDeviceListErrorHandle("")
			return
		}
	}
}

func (d *Detector) boardExists(deviceID string) bool {
	_, exists := d.boards[deviceID]
	return exists
}
func (d *Detector) boardExistsSync(deviceID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.boardExists(deviceID)
}
