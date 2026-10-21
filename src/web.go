// содержит функции, связанные с отправкой и обработкой сообщений
package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/tjgq/ticker"
)

var (
	websocketUpgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
)

// Менеджер используется для контроля и хранения соединений с клиентами
type WebSocketManager struct {
	// обработчики сообщений (событий)
	handlers map[string]EventHandler
	// список соединений
	connections *syncLenMap
	// отпраляет сигнал, когда нужно обновить устройства для всех
	updateTicker ticker.Ticker
}

// Инициализация менеджера
func NewWebSocketManager() *WebSocketManager {
	var m WebSocketManager
	m.connections = initSyncLenMap()
	m.handlers = make(map[string]EventHandler)
	m.setupEventHandlers()
	m.updateTicker = *ticker.New(updateListTime)
	m.updateTicker.Start()
	go m.updater()
	return &m
}

func (m *WebSocketManager) hasMultipleConnections() bool {
	return (m.connections.Len() > 1)
}

// инициализация обработчиков событий
func (m *WebSocketManager) setupEventHandlers() {
	m.handlers[GetListMsg] = GetList
	m.handlers["get-board-profiles"] = GetBoardProfiles
	m.handlers["set-board-profile"] = SetBoardProfile
	m.handlers[FlashStartMsg] = FlashStart
	m.handlers[MSBinStartMsg] = FlashStart
	m.handlers[FlashBinaryBlockMsg] = FlashBinaryBlock
	m.handlers[GetMaxFileSizeMsg] = GetMaxFileSize
	m.handlers[SerialConnectMsg] = SerialConnect
	m.handlers[SerialDisconnectMsg] = SerialDisconnect
	m.handlers[SerialSendMsg] = SerialSend
	m.handlers[SerialChangeBaudMsg] = SerialChangeBaud
	m.handlers[MSGetAddressMsg] = MSGetAddress
	m.handlers[MSPingMsg] = MSPing
	m.handlers[MSResetMsg] = MSReset
	m.handlers[MSGetMetaDataMsg] = MSGetMetaData
	m.handlers[MSGetAddressAndMetaMsg] = MSGetAddressAndMeta
	m.handlers[MSGetFirmwareMsg] = GetMsFirmwareStart
	m.handlers[GetFirmwareMsg] = GetFirmwareStart
	m.handlers[MSGetFirmwareNextBlockMsg] = GetFirmwareNextBlock
	m.handlers[MSGetConnectedBoardsMsg] = MSGetConnectedBoards
	m.handlers[requestPackMsg] = RequestPack
	m.handlers[pingMsg] = Ping
	m.handlers[resetMsg] = Reset
	m.handlers[GetMetaDataMsg] = GetMetaData
}

// обработка нового соединения
func (m *WebSocketManager) serveWS(w http.ResponseWriter, r *http.Request) {
	printLog("New connection")
	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	c := NewWebSocket(conn, getListCooldownDuration, m, maxThreadsPerClient)
	m.addClient(c)
	defer func() {
		m.updateTicker.Stop()
		UpdateList(c, m)
		m.updateTicker.Start()
	}()
	go m.writerHandler(c)
	go m.readerHandler(c)
}

// добавление нового клиента
func (m *WebSocketManager) addClient(c *WebSocketConnection) {
	m.connections.Add(c, true)
}

// удаление клиента
// если устройство не прошилось, то оно продолжит прошиваться и затем разблокируется
func (m *WebSocketManager) removeClient(c *WebSocketConnection) {
	printLog("remove client")
	if m.connections.Remove(c) {
		c.wsc.Close()
		c.closeChan()
	}
}

// обработчик входящих сообщений
func (m *WebSocketManager) readerHandler(c *WebSocketConnection) {
	defer func() {
		m.removeClient(c)
	}()

	c.wsc.SetReadLimit(int64(maxMsgSize))
	for {
		if c.isClosedChan() {
			return
		}
		msgType, payload, err := c.wsc.ReadMessage()
		if err != nil {
			printLog("reader: removed")
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error reading message: %v", err)
			}
			break
		}

		var event Event
		if msgType == websocket.BinaryMessage {
			event.Type = FlashBinaryBlockMsg
			event.Payload = payload
		} else {
			err := json.Unmarshal(payload, &event)
			if err != nil {
				errorHandler(ErrUnmarshal, c)
				continue
			}
		}
		c.addQuerry(event)
	}
}

// обработчик исходящих сообщений
func (m *WebSocketManager) writerHandler(c *WebSocketConnection) {
	defer func() {
		printLog("writer: removed")
		m.removeClient(c)
	}()
	for {
		outgoing, isOpen := <-c.outgoingMsg
		if !isOpen {
			return
		}

		// некоторые сообщения нужно отправить всем клиентам
		/// TODO: действительно ли это хорошее решение? Отправять все сообщения через одного клиента? Может быть лучше реализовать отдельный метод для такого рода сообщений?
		if outgoing.toAll {
			m.connections.Range(func(conn *WebSocketConnection, value bool) {
				if conn != c {
					var curOutgoing OutgoingEventMessage
					curOutgoing.event = outgoing.event
					curOutgoing.toAll = false
					conn.outgoingMsg <- curOutgoing
				}
			})
		}
		// отправить одному клиенту
		if outgoing.event.Type == "" {
			// отправка бинарных сообщений
			err := c.wsc.WriteMessage(websocket.BinaryMessage, outgoing.event.Payload)
			if err != nil {
				log.Println("Writing binary error:", err.Error())
				return
			}
		} else {
			// отправка JSON сообщений
			err := c.wsc.WriteJSON(outgoing.event)
			printLog("writer", outgoing.event.Type)
			if err != nil {
				log.Println("Writing JSON error:", err.Error())
				return
			}
		}
	}
}

func (m *WebSocketManager) updater() {
	for {
		<-m.updateTicker.C
		if alwaysUpdate || m.connections.Len() > 0 {
			printLog("update")
			UpdateList(nil, m)
		}
	}
}

func (m *WebSocketManager) sendMessageToAll(msgType string, payload any) {
	m.connections.Range(func(connection *WebSocketConnection, value bool) {
		connection.sendOutgoingEventMessage(msgType, payload, false)
	})
}

func UpdateList(c *WebSocketConnection, m *WebSocketManager) {
	sendToAll := c == nil

	// замораживаем блокировки
	if sendToAll {
		m.connections.Range(func(connection *WebSocketConnection, value bool) {
			connection.getListCooldown.freeze()
		})
	} else {
		c.getListCooldown.freeze()
	}
	// запускаем временные блокировки
	defer func() {
		if sendToAll {
			m.connections.Range(func(connection *WebSocketConnection, value bool) {
				connection.getListCooldown.start()
			})
		} else {
			c.getListCooldown.start()
		}
	}()
	// отправляем все устройства клиенту
	// отправляем всем клиентам изменения в устройстве, если таковые имеются
	// отправляем всем остальным клиентам только новые устройства

	_, _, devicesInList := detector.Update()

	if !sendToAll {
		for deviceID, device := range devicesInList {
			SendDevice(deviceID, device, false, c)
		}
	}
	/*
		Отправка информации о новых устройствах,
		об изменении старых и их удалении.
	*/
	for {
		if boardWithAction, exists := detector.PopFrontActionSync(); exists {
			dev := boardWithAction.board
			if dev != nil {
				dev.Mu.Lock()
			}
			//TODO: в обоих случаях происходит тоже самое, просто используется разный синтаксис, следует придумать как это объединить
			if sendToAll {
				switch boardWithAction.action {
				case PORT_UPDATE:
					m.sendMessageToAll(DeviceUpdatePortMsg, newDeviceUpdatePortMessage(boardWithAction.board, boardWithAction.boardID))
				case ADD:
					m.sendMessageToAll(dev.Board.GetWebMessageType(), dev.Board.GetWebMessage(dev.TypeDesc.Name, boardWithAction.boardID))
				case DELETE:
					m.sendMessageToAll(DeviceUpdateDeleteMsg, newDeviceUpdateDeleteMessage(boardWithAction.boardID))
				default:
					printLog("Warning! Unknown action with board!", boardWithAction.action)
				}
			} else {
				switch boardWithAction.action {
				case PORT_UPDATE:
					DeviceUpdatePort(boardWithAction.boardID, boardWithAction.board, c)
				case ADD:
					SendDevice(boardWithAction.boardID, boardWithAction.board, true, c)
				case DELETE:
					DeviceUpdateDelete(boardWithAction.boardID, c)
				default:
					printLog("Warning! Unknown action with board!", boardWithAction.action)
				}
			}
			if dev != nil {
				dev.Mu.Unlock()
			}
		} else {
			break
		}
	}
}
