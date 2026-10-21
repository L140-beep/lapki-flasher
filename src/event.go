// обработка и отправка сообщений
package main

import (
	b64 "encoding/base64"
	"encoding/json"
	"log"
	"strings"

	"github.com/polyus-nt/ms1-go/pkg/ms1"
)

// обработчик события
type EventHandler func(event Event, c *WebSocketConnection) error

// Общий вид для всех сообщений, как от клиента так и от сервера
// (исключение: бинарные данные от клиента, но все равно они приводятся сервером к этой структуре)
type Event struct {
	// Тип сообщения (flash-start, get-list и т.д.)
	Type string `json:"type"`
	// Параметры сообщения, не все сообщения обязаны иметь параметры
	Payload json.RawMessage `json:"payload"`
}

type DeviceMessage struct {
	ProfileID        int    `json:"profileID"`
	ManuallyAssigned bool   `json:"manuallyAssigned"`
	ID               string `json:"deviceID"`
	Name             string `json:"name,omitempty"`
	Controller       string `json:"controller,omitempty"`
	Programmer       string `json:"programmer,omitempty"`
	PortName         string `json:"portName,omitempty"`
	SerialID         string `json:"serialID,omitempty"`
}

type MSDeviceMessage struct {
	ID        string    `json:"deviceID"`
	Name      string    `json:"name,omitempty"`
	PortNames [4]string `json:"portNames,omitempty"`
}

type BlgMbDeviceMessage struct {
	ID       string `json:"deviceID"`
	Name     string `json:"name,omitempty"`
	SerialID string `json:"serialID,omitempty"`
	Version  string `json:"version,omitempty"`
}

// минимальная информация об устройстве
type SimpleDeviceMessage struct {
	ID   string `json:"deviceID"`
	Name string `json:"name,omitempty"`
}

// тип данных для flash-start (для arduino-подобных устройств)
type FlashStartMessage struct {
	ID       string `json:"deviceID"`
	FileSize int    `json:"fileSize"` // размер прошивки
}

// тип данных для ms-bin-start (для МС-ТЮК)
type MSBinStartMessage struct {
	ID           string `json:"deviceID"`
	FileSize     int    `json:"fileSize"`     // размер прошивки
	Address      string `json:"address"`      // киберген
	Verification bool   `json:"verification"` // если true, то загрузчик потратит дополнительное время на проверку прошивки
}

type DeviceUpdateDeleteMessage struct {
	ID string `json:"deviceID"`
}

type DeviceUpdatePortMessage struct {
	ID       string `json:"deviceID"`
	PortName string `json:"portName"`
}

type MaxFileSizeMessage struct {
	Size int `json:"size"`
}

// тип данных для serial-connect и serial-change-baud
type SerialBaudMessage struct {
	ID   string `json:"deviceID"`
	Baud int    `json:"baud"`
}

type SerialDisconnectMessage struct {
	ID string `json:"deviceID"`
}

type DeviceCommentCodeMessage struct {
	ID      string `json:"deviceID"`
	Code    int    `json:"code"`
	Comment string `json:"comment"`
}

type DeviceCodeMessage struct {
	ID   string `json:"deviceID"`
	Code int    `json:"code"`
}

// тип данных для serial-device-read и serial-send
type SerialMessage struct {
	ID  string `json:"deviceID"`
	Msg string `json:"msg"`
}

type MSAddressMessage struct {
	ID      string `json:"deviceID"`
	Address string `json:"address"`
}

type MSGetAddressMessage struct {
	ID string `json:"deviceID"`
}

type DeviceIdMessage struct {
	ID string `json:"deviceID"`
}

type MetaSubMessage struct {
	RefBlHw       string `json:"RefBlHw"`       // Описывает физическое окружение контроллера (плату)
	RefBlFw       string `json:"RefBlFw"`       // Указывает на версию прошивки загрузчика
	RefBlUserCode string `json:"RefBlUserCode"` //
	RefBlChip     string `json:"RefBlChip"`     // Указывает на контроллер, здесь то, что нужно для компиляции прошивки
	RefBlProtocol string `json:"RefBlProtocol"` // Описывает возможности протокола загрузчика
	RefCgHw       string `json:"RefCgHw"`       // Указывает на аппаратное исполнение
	RefCgFw       string `json:"RefCgFw"`       // Указывает на версию прошивки кибергена
	RefCgProtocol string `json:"RefCgProtocol"` // Указывает на возможности протокола кибергена
}

type MSMetaDataMessage struct {
	ID     string         `json:"deviceID"`
	Meta   MetaSubMessage `json:"meta"`
	MSType string         `json:"type"` // тип устройства (определяется по RefBlHw)
}

type MetaDataMessage struct {
	ID   string `json:"deviceID"`
	Meta string `json:"meta"`
}

type FlashBacktrackMsMessage struct {
	UploadStage string `json:"UploadStage"`

	NoPacks bool `json:"NoPacks"`

	CurPack    uint16 `json:"CurPack"`
	TotalPacks uint16 `json:"TotalPacks"`
}

type MSAddressAndMetaMessage struct {
	ID        string         `json:"deviceID"`
	Address   string         `json:"address"`
	MSType    string         `json:"type"` // тип устройства (определяется по RefBlHw)
	ErrorMsg  string         `json:"errorMsg"`
	ErrorCode int            `json:"errorCode"`
	Meta      MetaSubMessage `json:"meta"`
}

type MSOperationReportMessage struct {
	ID      string `json:"deviceID"`
	Address string `json:"address"`
	Comment string `json:"comment"`
	Code    int    `json:"code"`
}

type MSGetFirmwareMessage struct {
	ID        string `json:"deviceID"`
	Address   string `json:"address"`
	BlockSize int    `json:"blockSize"`
	RefBlChip string `json:"RefBlChip"` // не обязательный параметр, помагает установить кол-во фреймов в МК
}

type GetFirmwareMessage struct {
	ID        string `json:"deviceID"`
	BlockSize int    `json:"blockSize"`
}

type MSAddressesMessage struct {
	ID        string   `json:"deviceID"`
	Addresses []string `json:"addresses"`
}

type MSGetConnectedBacktrackMessage struct {
	Address string `json:"address"`
	Code    int    `json:"code"`
}

type FlashNexBlockMessage struct {
	ID string `json:"ID"`
}

type FlashDoneMessage struct {
	ID         string `json:"ID"`
	FlasherMsg string `json:"flasherMsg"`
}

// типы сообщений (событий)
const (
	// запрос на получение списка всех устройств
	GetListMsg = "get-list"
	// описание ардуино подобного устройства
	DeviceMsg = "device"
	// описание МС-ТЮК
	MSDeviceMsg = "ms-device"
	// описание кибермишки
	BlgMbDeviceMsg = "blg-mb-device"
	// запрос на прошивку устройства
	FlashStartMsg = "flash-start"
	// прошивка прошла успешна
	FlashDoneMsg = "flash-done"
	// запрос на следующий блок бинарных данных
	FlashNextBlockMsg = "flash-next-block"
	// сообщение, для отметки бинарных данных загружаемого файла прошивки, прикрепляется сервером к сообщению после получения данных бинарного типа
	FlashBinaryBlockMsg = "flash-block"
	// обратная связь от программы загрузки прошивки МС-ТЮК
	FlashBackTrackMs = "flash-backtrack-ms"
	// устройство удалено из списка
	DeviceUpdateDeleteMsg = "device-update-delete"
	// устройство поменяло порт
	DeviceUpdatePortMsg = "device-update-port"
	GetMaxFileSizeMsg   = "get-max-file-size"
	MaxFileSizeMsg      = "max-file-size"
	// устройства не найдены
	EmptyListMsg = "empty-list"
	// запрос на запуск монитора порта
	SerialConnectMsg = "serial-connect"
	// статус соединения с устройством (монитора порта)
	SerialConnectionStatusMsg = "serial-connection-status"
	// закрыть монитор порта
	SerialDisconnectMsg = "serial-disconnect"
	// запрос на отправку сообщения на устройство
	SerialSendMsg = "serial-send"
	// статус отправленного сообщения на устройство (удалось ли его отправить или нет)
	SerialSentStatusMsg = "serial-sent-status"
	// сообщение от устройства
	SerialDeviceReadMsg = "serial-device-read"
	// сменить бод
	SerialChangeBaudMsg = "serial-change-baud"
	// запрос на прошивку МС-ТЮК по адресу
	MSBinStartMsg = "ms-bin-start"
	// пинг МС-ТЮК по адресу
	MSPingMsg = "ms-ping"
	// результат выполнения команды пинг
	MSPingResultMsg = "ms-ping-result"
	// получение адреса из МС-ТЮК
	MSGetAddressMsg = "ms-get-address"
	// передача адреса из МС-ТЮК клиенту
	MSAddressMsg = "ms-address"
	// сброс МС-ТЮК
	MSResetMsg = "ms-reset"
	// результат выполнения сброса МС-ТЮК
	MSResetResultMsg = "ms-reset-result"
	// запрос на получение метаданных
	GetMetaDataMsg = "get-meta-data"
	// запрос на получение метаданных МС-ТЮК
	MSGetMetaDataMsg = "ms-get-meta-data"
	// сообщение с метаданными, предназначенными для клиента
	MetaDataMsg = "meta-data"
	// сообщение с метаданными, предназначенными для клиента (МС-ТЮК)
	MSMetaDataMsg = "ms-meta-data"
	// сообщение в случае, если не удалось извлечь метаданные по запросу клиента
	MetaDataErrorMsg = "meta-data-error"
	// Получение адреса и метаданных платы
	MSGetAddressAndMetaMsg = "ms-get-address-and-meta"
	// Результат выполнения команды ms-get-address-and-meta
	MSAddressAndMetaMsg = "ms-address-and-meta"
	// Запрос от клиента на выгрузку прошивки из платы МС-ТЮК
	MSGetFirmwareMsg = "ms-get-firmware"
	// Запрос от клиента на выгрузку прошивки из платы
	GetFirmwareMsg = "get-firmware"
	// Одобрение на запрос выгрузки прошивки
	MSGetFirmwareApproveMsg = "ms-get-firmware-approve"
	// Запрос от клиента на получение блока с бинарными данными
	MSGetFirmwareNextBlockMsg = "get-firmware-next-block"
	// Отчёт о завершении процесса выгрузки прошивки
	MSGetFirmwareFinishMsg = "ms-get-firmware-finish"
	// Одобрение на запрос выгрузки прошивки
	GetFirmwareApproveMsg = "get-firmware-approve"
	// Отчёт о завершении процесса выгрузки прошивки
	GetFirmwareFinishMsg = "get-firmware-finish"
	// Запрос от клиента на получение адресов, подключенных плат МС-ТЮК
	MSGetConnectedBoardsMsg = "ms-get-connected-boards"
	// Сообщение с подключенными адресами плат МС-ТЮК
	MSConnectedBoardsMsg = "ms-connected-boards"
	// Ошибка получения адресов подключенных плат
	MSGetConnectedBoardsErrorMsg = "ms-get-connected-boards-error"
	// Обратная связь процесса получения подключенных плат
	MSGetConnectedBoardsBackTrackMsg = "ms-get-connected-boards-backtrack"
	// запрос на выполнение операций по очереди
	requestPackMsg = "requests-pack"
	// пинг устройства по deviceID
	pingMsg = "ping"
	// ответ на пинг устройства
	pongMsg = "pong"
	// перезагрузка устройства
	resetMsg = "reset"
	// результат операции reset
	resetResultMsg = "reset-result"
	// сигнал клиенту перед началом передачи бинарных данных
	prepareForBinary = "ready-for-binary"
)

// отправить клиенту список всех устройств
func GetList(event Event, c *WebSocketConnection) error {
	printLog("get-list")

	// откладываем таймер, так как обновление все равно произойдёт для всех
	manager := c.Manager
	manager.updateTicker.Stop()
	defer manager.updateTicker.Start()

	if c.getListCooldown.isBlocked() {
		return ErrGetListCoolDown
	}
	UpdateList(c, nil)
	if detector.boardsNum() == 0 {
		c.sendOutgoingEventMessage(EmptyListMsg, nil, false)
	}
	return nil
}

// отправить клиенту описание устройства
// lastGetListDevice - дополнительная переменная, берётся только первое значение, остальные будут игнорироваться
func SendDevice(deviceID string, board *Device, toAll bool, c *WebSocketConnection) error {
	err := c.sendOutgoingEventMessage(board.Board.GetWebMessageType(), board.Board.GetWebMessage(board.TypeDesc.Name, deviceID), toAll)
	if err != nil {
		printLog("device() error:", err.Error())
	}
	return err
}

// сообщение о том, что порт обновлён
func DeviceUpdatePort(deviceID string, board *Device, c *WebSocketConnection) {
	c.sendOutgoingEventMessage(DeviceUpdatePortMsg, newDeviceUpdatePortMessage(board, deviceID), true)
}

// сообщение о том, что устройство удалено
func DeviceUpdateDelete(deviceID string, c *WebSocketConnection) {
	c.sendOutgoingEventMessage(DeviceUpdateDeleteMsg, newDeviceUpdateDeleteMessage(deviceID), true)
}

// подготовка к чтению файла с прошивкой и к его загрузке на устройство
func FlashStart(event Event, c *WebSocketConnection) error {
	log.Println("Flash-start")
	if c.IsBinChanBusySync() {
		//FIXME: на клиенте нужно не забыть обработать случай, когда ошибка приходит от выгрузки прошивки, а не от загрузки
		return ErrFlashNotFinished
	}
	var deviceID string
	var fileSize int
	var address string    // адрес, только для МС-ТЮК
	var verification bool // верификация, только для МС-ТЮК
	if event.Type == FlashStartMsg {
		var msg FlashStartMessage
		err := json.Unmarshal(event.Payload, &msg)
		if err != nil {
			return ErrUnmarshal
		}
		deviceID = msg.ID
		fileSize = msg.FileSize
	} else {
		var msg MSBinStartMessage
		err := json.Unmarshal(event.Payload, &msg)
		if err != nil {
			return ErrUnmarshal
		}
		deviceID = msg.ID
		fileSize = msg.FileSize
		address = msg.Address
		verification = msg.Verification
	}
	if fileSize < 1 {
		return ErrIncorrectFileSize
	}
	if fileSize > maxFileSize {
		return ErrFlashLargeFile
	}
	dev, exists := detector.GetBoardSync(deviceID)
	if !exists {
		return ErrFlashWrongID
	}
	check := func() error {
		// плата блокируется!!!
		// не нужно использовать sync функции внутри блока
		dev.Mu.Lock()
		defer dev.Mu.Unlock()
		if _, unknown := dev.Board.(*UnknownPort); unknown {
			return ErrNotSupported
		}
		updated := dev.Board.Update()
		if updated {
			if dev.Board.IsConnected() {
				DeviceUpdatePort(deviceID, dev, c)
			} else {
				detector.DeleteBoard(deviceID)
				DeviceUpdateDelete(deviceID, c)
				return ErrFlashDisconnected
			}
		}
		// FIXME:
		// Это условие возможно никогда не сработает из-за блокировки mutex.
		// Если два клиента попытаются прошить одно и то же устройство, то один из них будет ждать своей очереди.
		// Нужно подумать, стоит ли перенести это условие в другое место, или просто его убрать.
		if dev.IsFlashBlocked() {
			if c.FlashingDevId == deviceID {
				return ErrFlashNotFinished
			}
			return ErrFlashBlocked
		}
		boardToFlashName := strings.ToLower(dev.TypeDesc.Name)
		for _, boardName := range notSupportedBoards {
			if boardToFlashName == strings.ToLower(boardName) {
				c.sendOutgoingEventMessage(ErrNotSupported.Error(), boardName, false)
				return nil
			}
		}
		switch dev.Board.(type) {
		case *Arduino:
			if dev.SerialMonitor.isOpen() {
				return ErrFlashOpenSerialMonitor
			}
		case *MS1:
			if event.Type == MSBinStartMsg {
				if address != "" {
					dev.Board.(*MS1).address = address
				}
				dev.Board.(*MS1).verify = verification
			}
		}
		// блокировка устройства и клиента для прошивки, необходимо разблокировать после завершения прошивки
		// это sync функция, но она блокирует клиент, а не устройство
		c.SetFlashingBoard(dev, deviceID)
		c.FlashingBoard.SetLock(true)

		return nil
	}
	err := check()
	if err != nil {
		return err
	}
	FileWriter := newFlashFileWriter()
	FileWriter.Start(fileSize, dev.TypeDesc.FlashFileExtension)
	defer func() {
		if FileWriter != nil {
			FileWriter.Clear()
		} else {
			// Сообщение для дебага, если это сообщение появилось, то значит, что-то пошло не так
			println("WARNING! FileWriter is nil")
		}
		if c.FlashingBoard != nil {
			c.FlashingBoard.SetLockSync(false)
		} else {
			// Сообщение для дебага, если это сообщение появилось, то значит, что-то пошло не так
			println("WARNING! FlashingBoard is nil")
		}
		c.SetFlashingBoard(nil, "")
	}()
	FlashNextBlock(c)
	for {
		binData, isOpen := <-c.binDataChan
		if !isOpen {
			//TODO
			break
		}
		fileCreated, err := FileWriter.AddBlock(binData)
		if err != nil {
			return ErrFileWriter
		}
		if fileCreated {
			logger := make(chan any)
			go LogSend(c, logger)
			flasherMsg, err := dev.Board.Flash(FileWriter.GetFilePath(), logger)
			c.flasherMsg = flasherMsg
			if err != nil {
				return ErrAvrdude
			}
			err = c.sendOutgoingEventMessage(FlashDoneMsg, c.GetFlasherMessageSync(), false)
			c.SetFlasherMessageSync("")
			return err
		} else {
			FlashNextBlock(c)
		}
	}
	return nil
}

func LogSend(client *WebSocketConnection, logger chan any) {
	if client.FlashingBoard == nil || logger == nil {
		return
	}
	switch client.FlashingBoard.Board.(type) {
	case *MS1:
		for log := range logger {
			client.sendOutgoingEventMessage(FlashBackTrackMs, log, false)
		}
		printLog("firmware logging is over")
	}
}

// принятие блока с бинарными данными файла
func FlashBinaryBlock(event Event, c *WebSocketConnection) error {
	// FIXME: cделать функцию sync?
	if !c.IsBinChanBusySync() {
		return ErrFlashNotStarted
	}
	c.binDataChan <- event.Payload
	return nil
}

// запрос на следующий блок с бинаными данными файла
func FlashNextBlock(c *WebSocketConnection) {
	c.sendOutgoingEventMessage(FlashNextBlockMsg, nil, false)
}

func newDeviceUpdatePortMessage(dev *Device, deviceID string) *DeviceUpdatePortMessage {
	boardMessage := DeviceUpdatePortMessage{
		deviceID,
		dev.Board.(*Arduino).portName,
	}
	return &boardMessage
}

func newDeviceUpdateDeleteMessage(deviceID string) *DeviceUpdateDeleteMessage {
	boardMessage := DeviceUpdateDeleteMessage{
		deviceID,
	}
	return &boardMessage
}

func GetMaxFileSize(event Event, c *WebSocketConnection) error {
	return c.sendOutgoingEventMessage(MaxFileSizeMsg, MaxFileSizeMessage{maxFileSize}, false)
}

func SerialConnect(event Event, c *WebSocketConnection) error {
	var msg SerialBaudMessage
	err := json.Unmarshal(event.Payload, &msg)
	if err != nil {
		SerialConnectionStatus(DeviceCommentCodeMessage{
			ID:      msg.ID,
			Code:    4,
			Comment: err.Error(),
		}, c)
		return err
	}
	dev, exists := detector.GetBoardSync(msg.ID)
	if !exists {
		DeviceUpdateDelete(msg.ID, c)
		SerialConnectionStatus(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: 2,
		}, c)
		return nil
	}
	if dev.isFake() {
		SerialConnectionStatus(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: 3,
		}, c)
		return nil
	}
	// плата блокируется!!!
	// не нужно использовать sync функции внутри блока
	dev.Mu.Lock()
	defer dev.Mu.Unlock()
	updated := dev.Board.Update()
	if updated {
		if dev.Board.IsConnected() {
			DeviceUpdatePort(msg.ID, dev, c)
		} else {
			detector.DeleteBoard(msg.ID)
			DeviceUpdateDelete(msg.ID, c)
			SerialConnectionStatus(DeviceCommentCodeMessage{
				ID:   msg.ID,
				Code: 2,
			}, c)
			return nil
		}
	}
	if _, isArduino := dev.Board.(*Arduino); isArduino && dev.IsFlashBlocked() {
		SerialConnectionStatus(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: 5,
		}, c)
		return nil
	}
	if dev.SerialMonitor.isOpen() {
		SerialConnectionStatus(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: 6,
		}, c)
		return nil
	}
	serialPort, err := openSerialPort(dev.Board.GetSerialPort(), msg.Baud)
	if err != nil {
		SerialConnectionStatus(DeviceCommentCodeMessage{
			ID:      msg.ID,
			Code:    1,
			Comment: err.Error(),
		}, c)
		return nil
	}
	SerialConnectionStatus(DeviceCommentCodeMessage{
		ID:   msg.ID,
		Code: 0,
	}, c)
	dev.SerialMonitor.set(serialPort, c, msg.Baud)
	go handleSerial(dev, msg.ID)
	return nil
}

func SerialConnectionStatus(status DeviceCommentCodeMessage, c *WebSocketConnection) {
	c.sendOutgoingEventMessage(SerialConnectionStatusMsg, status, false)
}

func SerialDisconnect(event Event, c *WebSocketConnection) error {
	var msg SerialDisconnectMessage
	err := json.Unmarshal(event.Payload, &msg)
	if err != nil {
		return err
	}
	board, exists := detector.GetBoardSync(msg.ID)
	// плата блокируется!!!
	// не нужно использовать sync функции внутри блока
	board.Mu.Lock()
	defer board.Mu.Unlock()
	if exists {
		if board.SerialMonitor.Client != c {
			SerialConnectionStatus(DeviceCommentCodeMessage{
				ID:   msg.ID,
				Code: 14,
			}, c)
			return nil
		}
		board.SerialMonitor.close()
		SerialConnectionStatus(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: 8,
		}, c)
	} else {
		DeviceUpdateDelete(msg.ID, c)
		SerialConnectionStatus(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: 2,
		}, c)
	}
	return nil
}

func SerialSend(event Event, c *WebSocketConnection) error {
	var msg SerialMessage
	err := json.Unmarshal(event.Payload, &msg)
	if err != nil {
		SerialSentStatus(DeviceCommentCodeMessage{
			ID:      msg.ID,
			Code:    4,
			Comment: err.Error(),
		}, c)
		return err
	}
	dev, exists := detector.GetBoardSync(msg.ID)
	if !exists {
		DeviceUpdateDelete(msg.ID, c)
		SerialSentStatus(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: 2,
		}, c)
		SerialConnectionStatus(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: 2,
		}, c)
		return nil
	}
	if !dev.isSerialMonitorOpenSync() {
		SerialSentStatus(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: 3,
		}, c)
		return nil
	}
	updated := dev.updateSync()
	if updated {
		if dev.isConnectedSync() {
			DeviceUpdatePort(msg.ID, dev, c)
		} else {
			detector.DeleteBoard(msg.ID)
			DeviceUpdateDelete(msg.ID, c)
			SerialSentStatus(DeviceCommentCodeMessage{
				ID:   msg.ID,
				Code: 2,
			}, c)
			return nil
		}
	}
	if dev.SerialMonitor.Client != c {
		SerialSentStatus(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: 5,
		}, c)
	}
	decoded, err := b64.StdEncoding.DecodeString(msg.Msg)
	if err != nil {
		SerialSentStatus(DeviceCommentCodeMessage{
			ID:      msg.ID,
			Code:    1,
			Comment: err.Error(),
		}, c)
		return nil
	}
	// см. handleSerial в serialMonitor.go
	dev.SerialMonitor.Write <- decoded
	return nil
}

func SerialSentStatus(status DeviceCommentCodeMessage, c *WebSocketConnection) {
	c.sendOutgoingEventMessage(SerialSentStatusMsg, status, false)
}

func SerialChangeBaud(event Event, c *WebSocketConnection) error {
	var msg SerialBaudMessage
	err := json.Unmarshal(event.Payload, &msg)
	if err != nil {
		SerialConnectionStatus(DeviceCommentCodeMessage{
			ID:      msg.ID,
			Code:    11,
			Comment: err.Error(),
		}, c)
		return err
	}
	dev, exists := detector.GetBoardSync(msg.ID)
	if !exists {
		DeviceUpdateDelete(msg.ID, c)
		SerialConnectionStatus(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: 2,
		}, c)
		return nil
	}
	if !dev.isConnectedSync() {
		SerialConnectionStatus(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: 12,
		}, c)
		return nil
	}
	if dev.getSerialMonitorClientSync() != c {
		SerialConnectionStatus(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: 13,
		}, c)
		return nil
	}
	if msg.Baud == dev.getSerialMonitorBaudSync() {
		SerialConnectionStatus(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: 15,
		}, c)
		return nil
	}
	// см. handleSerial в serialMonitor.go
	dev.SerialMonitor.ChangeBaud <- msg.Baud
	return nil
}

func MSPing(event Event, c *WebSocketConnection) error {
	var msg MSAddressMessage
	err := json.Unmarshal(event.Payload, &msg)
	if err != nil {
		MSPingResult(msg.ID, 4, err.Error(), c)
		return err
	}
	dev, exists := detector.GetBoardSync(msg.ID)
	if !exists {
		DeviceUpdateDelete(msg.ID, c)
		MSPingResult(msg.ID, 1, "", c)
		return nil
	}
	dev.Mu.Lock()
	defer dev.Mu.Unlock()
	board, isMS1 := dev.Board.(*MS1)
	if !isMS1 {
		MSPingResult(msg.ID, 3, "", c)
		return nil
	}
	updated := board.Update()
	if updated {
		if dev.Board.IsConnected() {
			// TODO
		} else {
			detector.DeleteBoard(msg.ID)
			DeviceUpdateDelete(msg.ID, c)
			MSPingResult(msg.ID, 1, "", c)
			return nil
		}
	}
	board.address = msg.Address
	err = board.Ping()
	if err != nil {
		MSPingResult(msg.ID, 2, err.Error(), c)
		return err
	}
	MSPingResult(msg.ID, 0, "", c)
	return nil
}

func MSGetAddress(event Event, c *WebSocketConnection) error {
	var msg MSGetAddressMessage
	err := json.Unmarshal(event.Payload, &msg)
	if err != nil {
		MSAddressSend(msg.ID, 4, err.Error(), c)
		return err
	}
	dev, exists := detector.GetBoardSync(msg.ID)
	if !exists {
		DeviceUpdateDelete(msg.ID, c)
		MSAddressSend(msg.ID, 1, "", c)
		return nil
	}
	dev.Mu.Lock()
	defer dev.Mu.Unlock()
	board, isMS1 := dev.Board.(*MS1)
	if !isMS1 {
		MSAddressSend(msg.ID, 3, "", c)
		return nil
	}
	updated := board.Update()
	if updated {
		if board.IsConnected() {
			// TODO
		} else {
			detector.DeleteBoard(msg.ID)
			DeviceUpdateDelete(msg.ID, c)
			MSAddressSend(msg.ID, 1, "", c)
			return nil
		}
	}
	address, err := board.getAddress()
	if err != nil {
		MSAddressSend(msg.ID, 2, err.Error(), c)
		return err
	}
	MSAddressSend(msg.ID, 0, address, c)
	return nil
}

func MSPingResult(deviceID string, code int, comment string, c *WebSocketConnection) {
	DeviceCommentCode(MSPingResultMsg, deviceID, code, comment, c)
}

func DeviceCommentCode(messageType string, deviceID string, code int, comment string, c *WebSocketConnection) {
	c.sendOutgoingEventMessage(messageType, DeviceCommentCodeMessage{
		ID:      deviceID,
		Code:    code,
		Comment: comment,
	}, false)
}

func MSAddressSend(deviceID string, code int, comment string, c *WebSocketConnection) {
	DeviceCommentCode(MSAddressMsg, deviceID, code, comment, c)
}

func MSReset(event Event, c *WebSocketConnection) error {
	var msg MSAddressMessage
	err := json.Unmarshal(event.Payload, &msg)
	if err != nil {
		MSResetSend(msg.ID, 4, err.Error(), c)
		return err
	}
	dev, exists := detector.GetBoardSync(msg.ID)
	if !exists {
		DeviceUpdateDelete(msg.ID, c)
		MSResetSend(msg.ID, 1, "", c)
		return nil
	}
	dev.Mu.Lock()
	defer dev.Mu.Unlock()
	board, isMS1 := dev.Board.(*MS1)
	if !isMS1 {
		MSResetSend(msg.ID, 3, "", c)
		return nil
	}
	updated := board.Update()
	if updated {
		if board.IsConnected() {
			// TODO
		} else {
			detector.DeleteBoard(msg.ID)
			DeviceUpdateDelete(msg.ID, c)
			MSResetSend(msg.ID, 1, "", c)
			return nil
		}
	}
	board.address = msg.Address
	err = board.Reset()
	if err != nil {
		MSResetSend(msg.ID, 2, err.Error(), c)
		return err
	}
	MSResetSend(msg.ID, 0, "", c)
	return nil
}

func MSResetSend(deviceID string, code int, comment string, client *WebSocketConnection) {
	DeviceCommentCode(MSResetResultMsg, deviceID, code, comment, client)
}

func MetaDataError(deviceID string, code int, comment string, client *WebSocketConnection) {
	DeviceCommentCode(MetaDataErrorMsg, deviceID, code, comment, client)
}

const (
	META_ERROR        = 1
	META_NO_DEVICE    = 2
	META_WRONG_DEVICE = 3
	META_JSON_ERROR   = 4
)

func MSGetMetaData(event Event, c *WebSocketConnection) error {
	var msg MSAddressMessage
	err := json.Unmarshal(event.Payload, &msg)
	if err != nil {
		MetaDataError(msg.ID, META_JSON_ERROR, err.Error(), c)
		return err
	}
	dev, exists := detector.GetBoardSync(msg.ID)
	if !exists {
		DeviceUpdateDelete(msg.ID, c)
		MetaDataError(msg.ID, META_NO_DEVICE, "", c)
		return nil
	}
	dev.Mu.Lock()
	defer dev.Mu.Unlock()
	board, isMS1 := dev.Board.(*MS1)
	if !isMS1 {
		MetaDataError(msg.ID, META_WRONG_DEVICE, "", c)
		return nil
	}
	updated := board.Update()
	if updated {
		if board.IsConnected() {
			// TODO
		} else {
			detector.DeleteBoard(msg.ID)
			DeviceUpdateDelete(msg.ID, c)
			MetaDataError(msg.ID, META_NO_DEVICE, "", c)
			return nil
		}
	}
	board.address = msg.Address
	value, err := board.GetMetaData()
	if err != nil {
		MetaDataError(msg.ID, META_ERROR, err.Error(), c)
		return err
	}
	meta, ok := value.(*ms1.Meta)
	if !ok {
		MetaDataError(msg.ID, META_ERROR, "метаданные не прошли проверку типа", c)
		return nil
	}
	c.sendOutgoingEventMessage(MSMetaDataMsg, MSMetaDataMessage{
		ID:     msg.ID,
		Meta:   metaToJSON(meta),
		MSType: getMSType(meta.RefBlHw),
	}, false)
	return nil
}

func GetMetaData(event Event, c *WebSocketConnection) error {
	var msg DeviceIdMessage
	err := json.Unmarshal(event.Payload, &msg)
	if err != nil {
		MetaDataError(msg.ID, META_JSON_ERROR, err.Error(), c)
		return err
	}
	dev, exists := detector.GetBoardSync(msg.ID)
	if !exists {
		DeviceUpdateDelete(msg.ID, c)
		MetaDataError(msg.ID, META_NO_DEVICE, "", c)
		return nil
	}
	dev.Mu.Lock()
	defer dev.Mu.Unlock()
	board := dev.Board
	updated := board.Update()
	if updated {
		if board.IsConnected() {
			// TODO
		} else {
			detector.DeleteBoard(msg.ID)
			DeviceUpdateDelete(msg.ID, c)
			MetaDataError(msg.ID, META_NO_DEVICE, "", c)
			return nil
		}
	}
	value, err := board.GetMetaData()
	if err != nil {
		MetaDataError(msg.ID, META_ERROR, err.Error(), c)
		return err
	}
	meta, ok := value.(string)
	if !ok {
		MetaDataError(msg.ID, META_ERROR, "метаданные не прошли проверку типа", c)
		return nil
	}
	c.sendOutgoingEventMessage(MetaDataMsg, MetaDataMessage{
		ID:   msg.ID,
		Meta: meta,
	}, false)
	return nil
}

func MSAddressAndMeta(msg MSAddressAndMetaMessage, c *WebSocketConnection) {
	c.sendOutgoingEventMessage(MSAddressAndMetaMsg, msg, false)
}

func MSGetAddressAndMeta(event Event, c *WebSocketConnection) error {
	const (
		NO_ERROR  = 0
		NO_ADDR   = 1
		NO_META   = 2
		NO_DEV    = 3
		WRONG_DEV = 4
	)
	var msg MSGetAddressMessage
	err := json.Unmarshal(event.Payload, &msg)
	if err != nil {
		MSAddressAndMeta(MSAddressAndMetaMessage{
			ID:        msg.ID,
			ErrorMsg:  err.Error(),
			ErrorCode: NO_ADDR,
			MSType:    "",
			Address:   "",
			Meta:      MetaSubMessage{},
		}, c)
		return err
	}
	dev, exists := detector.GetBoardSync(msg.ID)
	if !exists {
		DeviceUpdateDelete(msg.ID, c)
		MSAddressAndMeta(MSAddressAndMetaMessage{
			ID:        msg.ID,
			ErrorMsg:  "",
			ErrorCode: NO_DEV,
			MSType:    "",
			Address:   "",
			Meta:      MetaSubMessage{},
		}, c)
		return nil
	}
	dev.Mu.Lock()
	defer dev.Mu.Unlock()
	board, isMS1 := dev.Board.(*MS1)
	if !isMS1 {
		MSAddressAndMeta(MSAddressAndMetaMessage{
			ID:        msg.ID,
			ErrorMsg:  "",
			ErrorCode: WRONG_DEV,
			MSType:    "",
			Address:   "",
			Meta:      MetaSubMessage{},
		}, c)
		return nil
	}
	updated := board.Update()
	if updated {
		if !board.IsConnected() {
			detector.DeleteBoard(msg.ID)
			DeviceUpdateDelete(msg.ID, c)
			MSAddressAndMeta(MSAddressAndMetaMessage{
				ID:        msg.ID,
				ErrorMsg:  "",
				ErrorCode: NO_DEV,
				MSType:    "",
				Address:   "",
				Meta:      MetaSubMessage{},
			}, c)
		}
	}
	addr, meta, err := board.getAddressAndMeta()
	if err != nil {
		if addr == "" {
			MSAddressAndMeta(MSAddressAndMetaMessage{
				ID:        msg.ID,
				ErrorMsg:  err.Error(),
				ErrorCode: NO_ADDR,
				MSType:    "",
				Address:   "",
				Meta:      MetaSubMessage{},
			}, c)
		} else {
			MSAddressAndMeta(MSAddressAndMetaMessage{
				ID:        msg.ID,
				ErrorMsg:  err.Error(),
				ErrorCode: NO_META,
				MSType:    "",
				Address:   addr,
				Meta:      MetaSubMessage{},
			}, c)
		}
		return err
	}
	MSAddressAndMeta(MSAddressAndMetaMessage{
		ID:        msg.ID,
		ErrorMsg:  "",
		ErrorCode: NO_ERROR,
		MSType:    getMSType(meta.RefBlHw),
		Address:   addr,
		Meta:      metaToJSON(meta),
	}, c)
	return nil
}

const (
	GET_FIRMWARE_DONE                 = 0
	GET_FIRMWARE_NO_DEV               = 1
	GET_FIRMWARE_WRONG_DEV            = 2
	GET_FIRMWARE_ERROR                = 3
	GET_FIRMWARE_CLIENT_BUSY          = 4
	GET_FIRMWARE_DEVICE_BUSY          = 5
	GET_FIRMWARE_INCORRECT_BLOCK_SIZE = 6
	GET_FIRMWARE_TIMEOUT              = 7
)

func MSGetFirmwareFinish(msg MSOperationReportMessage, c *WebSocketConnection) {
	c.sendOutgoingEventMessage(MSGetFirmwareFinishMsg, msg, false)
}

// обработка запроса на выгрузку прошивки из устройства (МС-ТЮК)
func GetMsFirmwareStart(event Event, c *WebSocketConnection) error {
	var msg MSGetFirmwareMessage
	err := json.Unmarshal(event.Payload, &msg)
	if err != nil {
		MSGetFirmwareFinish(
			MSOperationReportMessage{
				Comment: err.Error(),
				Code:    GET_FIRMWARE_ERROR,
			}, c)
		return err
	}
	if c.IsBinChanBusySync() {
		MSGetFirmwareFinish(
			MSOperationReportMessage{
				ID:      msg.ID,
				Address: msg.Address,
				Code:    GET_FIRMWARE_CLIENT_BUSY,
			}, c)
		return nil
	}
	if msg.BlockSize < 1 {
		MSGetFirmwareFinish(
			MSOperationReportMessage{
				ID:      msg.ID,
				Address: msg.Address,
				Code:    GET_FIRMWARE_INCORRECT_BLOCK_SIZE,
			}, c)
		return nil
	}
	dev, exists := detector.GetBoardSync(msg.ID)
	if !exists {
		MSGetFirmwareFinish(
			MSOperationReportMessage{
				ID:      msg.ID,
				Address: msg.Address,
				Code:    GET_FIRMWARE_NO_DEV,
			}, c)
		return nil
	}
	// плата блокируется!!!
	// не нужно использовать sync функции внутри блока
	dev.Mu.Lock()
	defer dev.Mu.Unlock()
	updated := dev.Board.Update()
	if updated {
		if dev.Board.IsConnected() {
			// TODO
		} else {
			detector.DeleteBoard(msg.ID)
			DeviceUpdateDelete(msg.ID, c)
			MSGetFirmwareFinish(MSOperationReportMessage{
				ID:      msg.ID,
				Address: msg.Address,
				Code:    GET_FIRMWARE_NO_DEV,
			}, c)
			return nil
		}
	}
	if dev.IsFlashBlocked() {
		MSGetFirmwareFinish(
			MSOperationReportMessage{
				ID:      msg.ID,
				Address: msg.Address,
				Code:    GET_FIRMWARE_DEVICE_BUSY,
			}, c)
		return nil
	}
	// блокировка устройства и клиента для выгрузки, необходимо разблокировать после завершения выгрузки
	c.SetFlashingBoard(dev, msg.ID)
	c.FlashingBoard.SetLock(true)
	transmission := newDataTransmission()
	defer func() {
		if c.GetFlashingBoardSync() != nil {
			c.FlashingBoard.SetLock(false)
		} else {
			// Сообщение для дебага, если это сообщение появилось, то значит, что-то пошло не так
			println("WARNING! FlashingBoard is nil")
		}
		c.SetFlashingBoard(nil, "")
		transmission.Clear()
	}()

	c.sendOutgoingEventMessage(MSGetFirmwareApproveMsg, MSAddressMessage{
		ID:      msg.ID,
		Address: msg.Address,
	}, false)

	board := dev.Board.(*MS1)
	logger := make(chan any)
	go LogSend(c, logger)
	bytes, err := board.getFirmware(msg.Address, logger, msg.RefBlChip)
	if err != nil {
		close(logger)
		MSGetFirmwareFinish(MSOperationReportMessage{
			ID:      msg.ID,
			Address: msg.Address,
			Comment: err.Error(),
			Code:    GET_FIRMWARE_ERROR,
		}, c)
		return err
	}
	transmission.set(bytes, msg.BlockSize)
	c.sendOutgoingEventMessage(prepareForBinary, nil, false)
	for {
		if transmission.isFinish() {
			c.binDataChan <- []byte{}
			MSGetFirmwareFinish(MSOperationReportMessage{
				ID:      msg.ID,
				Address: msg.Address,
				Code:    GET_FIRMWARE_DONE,
			}, c)
			return nil
		}
		c.binDataChan <- transmission.popBlock()
	}
}

// обработка запроса на выгрузку прошивки из устройства
func GetFirmwareStart(event Event, c *WebSocketConnection) error {
	var msg GetFirmwareMessage
	err := json.Unmarshal(event.Payload, &msg)
	if err != nil {
		DeviceCommentCode(GetFirmwareFinishMsg, msg.ID, GET_FIRMWARE_ERROR, err.Error(), c)
		return err
	}
	if c.IsBinChanBusySync() {
		DeviceCommentCode(GetFirmwareFinishMsg, msg.ID, GET_FIRMWARE_CLIENT_BUSY, "", c)
		return nil
	}
	if msg.BlockSize < 1 {
		DeviceCommentCode(GetFirmwareFinishMsg, msg.ID, GET_FIRMWARE_INCORRECT_BLOCK_SIZE, "", c)
		return nil
	}
	dev, exists := detector.GetBoardSync(msg.ID)
	if !exists {
		DeviceCommentCode(GetFirmwareFinishMsg, msg.ID, GET_FIRMWARE_NO_DEV, "", c)
		return nil
	}
	// плата блокируется!!!
	// не нужно использовать sync функции внутри блока
	dev.Mu.Lock()
	defer dev.Mu.Unlock()
	updated := dev.Board.Update()
	if updated {
		if dev.Board.IsConnected() {
			// TODO
		} else {
			detector.DeleteBoard(msg.ID)
			DeviceUpdateDelete(msg.ID, c)
			DeviceCommentCode(GetFirmwareFinishMsg, msg.ID, GET_FIRMWARE_NO_DEV, "", c)
			return nil
		}
	}
	if dev.IsFlashBlocked() {
		DeviceCommentCode(GetFirmwareFinishMsg, msg.ID, GET_FIRMWARE_DEVICE_BUSY, "", c)
		return nil
	}
	// блокировка устройства и клиента для выгрузки, необходимо разблокировать после завершения выгрузки
	c.SetFlashingBoard(dev, msg.ID)
	c.FlashingBoard.SetLock(true)
	transmission := newDataTransmission()
	defer func() {
		if c.GetFlashingBoardSync() != nil {
			c.FlashingBoard.SetLock(false)
		} else {
			// Сообщение для дебага, если это сообщение появилось, то значит, что-то пошло не так
			println("WARNING! FlashingBoard is nil")
		}
		c.SetFlashingBoard(nil, "")
		transmission.Clear()
	}()

	c.sendOutgoingEventMessage(GetFirmwareApproveMsg, DeviceIdMessage{
		ID: msg.ID,
	}, false)

	board := dev.Board.(*BlgMb)
	bytes, err := board.Extract()
	if err != nil {
		DeviceCommentCode(GetFirmwareFinishMsg, msg.ID, GET_FIRMWARE_ERROR, err.Error(), c)
		return err
	}
	transmission.set(bytes, msg.BlockSize)
	c.sendOutgoingEventMessage(prepareForBinary, nil, false)
	for {
		if transmission.isFinish() {
			c.binDataChan <- []byte{}
			DeviceCommentCode(GetFirmwareFinishMsg, msg.ID, GET_FIRMWARE_DONE, "", c)
			return nil
		}
		c.binDataChan <- transmission.popBlock()
	}
}

func GetFirmwareNextBlock(event Event, c *WebSocketConnection) error {
	if !c.IsBinChanBusySync() {
		//FIXME: на клиенте нужно не забыть обработать случай, когда ошибка приходит от выгрузки прошивки, а не от загрузки
		return ErrFlashNotStarted
	}
	bin := <-c.binDataChan
	if len(bin) == 0 {
		return nil
	}
	c.sendBinaryMessage(bin, false)
	return nil
}

func MSConnectedBoards(addresses MSAddressesMessage, client *WebSocketConnection) {
	client.sendOutgoingEventMessage(MSConnectedBoardsMsg, addresses, false)
}

func MSGetConnectedBoardsError(report DeviceCommentCodeMessage, client *WebSocketConnection) {
	client.sendOutgoingEventMessage(MSGetConnectedBoardsErrorMsg, report, false)
}

func MSGetConnectedBoardsBacktrack(report MSGetConnectedBacktrackMessage, client *WebSocketConnection) {
	client.sendOutgoingEventMessage(MSGetConnectedBoardsBackTrackMsg, report, false)
}

func MSGetConnectedBoards(event Event, c *WebSocketConnection) error {
	const (
		GET_BOARDS_ERROR              = 1
		GET_BOARDS_ERROR_NO_DEVICE    = 2
		GET_BOARDS_ERROR_WRONG_DEVICE = 3
	)
	var msg MSAddressesMessage
	err := json.Unmarshal(event.Payload, &msg)
	if err != nil {
		MSGetConnectedBoardsError(DeviceCommentCodeMessage{
			Code:    GET_BOARDS_ERROR,
			Comment: err.Error(),
		}, c)
		return err
	}
	dev, exists := detector.GetBoardSync(msg.ID)
	if !exists {
		DeviceUpdateDelete(msg.ID, c)
		MSGetConnectedBoardsError(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: GET_BOARDS_ERROR_NO_DEVICE,
		}, c)
		return nil
	}
	dev.Mu.Lock()
	defer dev.Mu.Unlock()
	board, isMS1 := dev.Board.(*MS1)
	if !isMS1 {
		MSGetConnectedBoardsError(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: GET_BOARDS_ERROR_WRONG_DEVICE,
		}, c)
		return nil
	}
	updated := board.Update()
	if updated {
		if board.IsConnected() {
			// TODO
		} else {
			detector.DeleteBoard(msg.ID)
			DeviceUpdateDelete(msg.ID, c)
			MSGetConnectedBoardsError(DeviceCommentCodeMessage{
				ID:   msg.ID,
				Code: GET_BOARDS_ERROR_NO_DEVICE,
			}, c)
			return nil
		}
	}
	connectedBoards, err := board.getConnectedBoards(msg.Addresses, c)
	if err != nil {
		MSGetConnectedBoardsError(DeviceCommentCodeMessage{
			ID:      msg.ID,
			Code:    GET_BOARDS_ERROR,
			Comment: err.Error(),
		}, c)
		return err
	}
	MSConnectedBoards(MSAddressesMessage{
		ID:        msg.ID,
		Addresses: connectedBoards,
	}, c)
	return nil
}

func RequestPack(event Event, c *WebSocketConnection) error {
	var requests []Event
	err := json.Unmarshal(event.Payload, &requests)
	if err != nil {
		return ErrUnmarshal
	}
	for _, req := range requests {
		c.handleEvent(req)
	}
	return nil
}

//TODO: сделать функции reset и ping общими для МС-ТЮК

func Reset(event Event, c *WebSocketConnection) error {
	const (
		RESET_OK  = 0
		NO_DEV    = 1
		RESET_ERR = 2
		WRONG_DEV = 3
		JSON_ERR  = 4
	)
	resetResult := func(resetResultMessage DeviceCommentCodeMessage) {
		c.sendOutgoingEventMessage(resetResultMsg, resetResultMessage, false)
	}
	var msg DeviceIdMessage
	err := json.Unmarshal(event.Payload, &msg)
	if err != nil {
		resetResult(DeviceCommentCodeMessage{
			Code:    JSON_ERR,
			Comment: err.Error(),
		})
		return err
	}
	dev, exists := detector.GetBoardSync(msg.ID)
	if !exists {
		DeviceUpdateDelete(msg.ID, c)
		resetResult(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: NO_DEV,
		})
		return nil
	}
	dev.Mu.Lock()
	defer dev.Mu.Unlock()
	updated := dev.Board.Update()
	if updated {
		if dev.Board.IsConnected() {
			DeviceUpdatePort(msg.ID, dev, c)
		} else {
			detector.DeleteBoard(msg.ID)
			DeviceUpdateDelete(msg.ID, c)
			resetResult(DeviceCommentCodeMessage{
				ID:   msg.ID,
				Code: NO_DEV,
			})
			return nil
		}
	}
	err = dev.Board.Reset()
	if err != nil {
		resetResult(DeviceCommentCodeMessage{
			ID:      msg.ID,
			Code:    RESET_ERR,
			Comment: err.Error(),
		})
		return err
	}
	resetResult(DeviceCommentCodeMessage{
		ID:   msg.ID,
		Code: RESET_OK,
	})
	return nil
}

func Ping(event Event, c *WebSocketConnection) error {
	const (
		PONG      = 0
		NO_DEV    = 1
		NO_PONG   = 2
		WRONG_DEV = 3
		JSON_ERR  = 4
	)
	pong := func(pongMessage DeviceCommentCodeMessage) {
		c.sendOutgoingEventMessage(pongMsg, pongMessage, false)
	}
	var msg DeviceIdMessage
	err := json.Unmarshal(event.Payload, &msg)
	if err != nil {
		pong(DeviceCommentCodeMessage{
			Code:    JSON_ERR,
			Comment: err.Error(),
		})
		return err
	}
	dev, exists := detector.GetBoardSync(msg.ID)
	if !exists {
		DeviceUpdateDelete(msg.ID, c)
		pong(DeviceCommentCodeMessage{
			ID:   msg.ID,
			Code: NO_DEV,
		})
		return nil
	}
	dev.Mu.Lock()
	defer dev.Mu.Unlock()
	updated := dev.Board.Update()
	if updated {
		if dev.Board.IsConnected() {
			DeviceUpdatePort(msg.ID, dev, c)
		} else {
			detector.DeleteBoard(msg.ID)
			DeviceUpdateDelete(msg.ID, c)
			pong(DeviceCommentCodeMessage{
				ID:   msg.ID,
				Code: NO_DEV,
			})
			return nil
		}
	}
	err = dev.Board.Ping()
	if err != nil {
		pong(DeviceCommentCodeMessage{
			ID:      msg.ID,
			Code:    NO_PONG,
			Comment: err.Error(),
		})
		return err
	}
	pong(DeviceCommentCodeMessage{
		ID:   msg.ID,
		Code: PONG,
	})
	return nil
}
