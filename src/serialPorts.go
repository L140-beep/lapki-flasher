package main

import (
	"encoding/json"
	"errors"
	"strings"

	serial "go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// UnknownPort is a serial port for which no board profile has been selected.
type UnknownPort struct {
	portName  string
	serialID  string
	vendorID  string
	productID string
}

func (p *UnknownPort) GetSerialPort() string     { return p.portName }
func (p *UnknownPort) IsConnected() bool         { return p.portName != NOT_FOUND }
func (p *UnknownPort) Update() bool              { return updateManualPort(&p.portName) }
func (p *UnknownPort) GetWebMessageType() string { return "unknown-device" }
func (p *UnknownPort) GetWebMessage(name, id string) any {
	return struct {
		ID     string `json:"deviceID"`
		Name   string `json:"name"`
		Port   string `json:"portName"`
		Serial string `json:"serialID"`
		VID    string `json:"vendorID"`
		PID    string `json:"productID"`
	}{id, name, p.portName, p.serialID, p.vendorID, p.productID}
}
func (p *UnknownPort) Flash(string, chan any) (string, error) { return "", ErrNotSupported }
func (p *UnknownPort) Ping() error                            { return ErrNotSupported }
func (p *UnknownPort) Reset() error                           { return ErrNotSupported }
func (p *UnknownPort) GetMetaData() (any, error)              { return nil, ErrNotSupported }

func updateManualPort(port *string) bool {
	ports, err := serial.GetPortsList()
	if err != nil {
		return false
	} // A failed scan is not evidence of a disconnect.
	for _, name := range ports {
		if name == *port {
			return false
		}
	}
	changed := *port != NOT_FOUND
	*port = NOT_FOUND
	return changed
}

func detectAllBoards(templates []BoardTemplate) map[string]*Device {
	boards := detectBoards(templates)
	if boards == nil {
		boards = make(map[string]*Device)
	}
	ports, err := serial.GetPortsList()
	if err != nil {
		printLog("Can't enumerate serial ports:", err)
		return boards
	}
	details, err := enumerator.GetDetailedPortsList()
	if err != nil {
		printLog("Can't read serial port metadata:", err)
	}
	metadata := make(map[string]*enumerator.PortDetails)
	for _, detail := range details {
		metadata[serialPortKey(detail.Name)] = detail
	}
	addUnknownPorts(boards, filterBluetoothPorts(ports, metadata, bluetoothPortNames()), metadata)
	return boards
}

// Filter by OS identity as well as descriptive metadata. Non-USB serial ports
// without metadata remain available for manual board selection.
func filterBluetoothPorts(ports []string, details map[string]*enumerator.PortDetails, excluded map[string]bool) []string {
	filtered := make([]string, 0, len(ports))
	for _, port := range ports {
		name := strings.ToLower(serialPortKey(port))
		if excluded[strings.ToUpper(port)] ||
			strings.HasPrefix(name, "/dev/rfcomm") ||
			strings.HasPrefix(name, "/dev/tty.bluetooth") {
			continue
		}
		if detail := details[serialPortKey(port)]; detail != nil && !detail.IsUSB &&
			strings.Contains(strings.ToLower(detail.Product), "bluetooth") {
			continue
		}
		filtered = append(filtered, port)
	}
	return filtered
}

// Windows supplies both kernel device names and PnP interface paths. Match
// identities rather than translated FriendlyName strings.
func isBluetoothDevicePath(path string) bool {
	path = strings.ToUpper(path)
	return strings.HasPrefix(path, `\DEVICE\BTHMODEM`) ||
		strings.HasPrefix(path, `BTHENUM\`) ||
		strings.HasPrefix(path, `BTHMODEM\`) ||
		strings.HasPrefix(path, `\\?\BTHENUM#`) ||
		strings.HasPrefix(path, `\\?\BTHMODEM#`)
}

// macOS exposes tty.* and cu.* aliases for the same physical serial port.
func serialPortKey(port string) string {
	return strings.Replace(port, "/dev/cu.", "/dev/tty.", 1)
}

func addUnknownPorts(boards map[string]*Device, ports []string, details map[string]*enumerator.PortDetails) {
	used := make(map[string]bool)
	for _, dev := range boards {
		// All four ports of an MS device belong to the recognized board.
		if ms, ok := dev.Board.(*MS1); ok {
			for _, port := range ms.portNames {
				used[serialPortKey(port)] = true
			}
		} else {
			used[serialPortKey(dev.Board.GetSerialPort())] = true
		}
	}
	for _, port := range ports {
		if port == "" || used[serialPortKey(port)] {
			continue
		}
		unknown := &UnknownPort{portName: port}
		if detail := details[serialPortKey(port)]; detail != nil {
			unknown.serialID, unknown.vendorID, unknown.productID = detail.SerialNumber, detail.VID, detail.PID
		}
		boards["serial:"+port] = newDevice(BoardTemplate{ID: -1, Name: "Неизвестная плата", Type: "unknown"}, unknown)
		used[serialPortKey(port)] = true
	}
}

type BoardProfile struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Bootloader identities are internal detection profiles, not user-selectable boards.
func (d *Detector) boardProfiles() []BoardProfile {
	bootloaders := make(map[int]bool)
	for _, template := range d.boardTemplates {
		if !template.IsArduinoDevice() {
			continue
		}
		var payload ArduinoPayload
		if json.Unmarshal(template.TypePayload, &payload) == nil && payload.BootloaderID >= 0 {
			bootloaders[payload.BootloaderID] = true
		}
	}
	profiles := make([]BoardProfile, 0)
	for _, template := range d.boardTemplates {
		if !template.IsArduinoDevice() || bootloaders[template.ID] {
			continue
		}
		var payload ArduinoPayload
		if json.Unmarshal(template.TypePayload, &payload) != nil || payload.Controller == "" || payload.Programmer == "" {
			continue
		}
		profiles = append(profiles, BoardProfile{template.ID, template.Name})
	}
	return profiles
}

func (d *Detector) manualProfile(id int) *BoardTemplate {
	for _, profile := range d.boardProfiles() {
		if profile.ID == id {
			for i := range d.boardTemplates {
				if d.boardTemplates[i].ID == id {
					return &d.boardTemplates[i]
				}
			}
		}
	}
	return nil
}

func deviceWithProfile(source *Device, profile BoardTemplate) *Device {
	var arduino *Arduino
	switch board := source.Board.(type) {
	case *Arduino:
		arduino = NewArduinoFromTemp(profile, board.portName, board.ardOS, board.serialID)
		arduino.manualPort = board.manualPort
	case *UnknownPort:
		arduino = NewArduinoFromTemp(profile, board.portName, ArduinoOS{}, board.serialID)
		arduino.manualPort = true
	default:
		return source
	}
	arduino.manuallyAssigned = true
	return newDevice(profile, arduino)
}

func (d *Detector) setBoardProfile(id string, profileID *int) (*Device, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	dev, exists := d.boards[id]
	source := d.detectedBoards[id]
	if !exists || source == nil {
		return nil, errors.New("Устройство отключено. Обновите список портов.")
	}
	dev.Mu.Lock()
	defer dev.Mu.Unlock()
	if dev.Flashing || dev.SerialMonitor.isOpen() {
		return nil, errors.New("Завершите прошивку и закройте монитор порта перед сменой платы.")
	}
	switch source.Board.(type) {
	case *Arduino, *UnknownPort:
	default:
		return nil, errors.New("Для этого устройства ручной выбор платы недоступен.")
	}
	selected := source
	if profileID != nil {
		profile := d.manualProfile(*profileID)
		if profile == nil {
			return nil, errors.New("Неизвестный профиль платы.")
		}
		selected = deviceWithProfile(source, *profile)
		d.manualProfiles[id] = *profileID
	} else {
		delete(d.manualProfiles, id)
	}
	dev.TypeDesc, dev.Board = selected.TypeDesc, selected.Board
	return dev, nil
}

func GetBoardProfiles(event Event, c *WebSocketConnection) error {
	detector.mu.Lock()
	profiles := detector.boardProfiles()
	detector.mu.Unlock()
	return c.sendOutgoingEventMessage("board-profiles", profiles, false)
}

func SetBoardProfile(event Event, c *WebSocketConnection) error {
	var msg struct {
		ID        string `json:"deviceID"`
		ProfileID *int   `json:"profileID"`
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(event.Payload, &msg) != nil || json.Unmarshal(event.Payload, &fields) != nil || fields["profileID"] == nil {
		return ErrUnmarshal
	}
	dev, err := detector.setBoardProfile(msg.ID, msg.ProfileID)
	if err != nil {
		return c.sendOutgoingEventMessage("board-profile-result", DeviceCommentCodeMessage{ID: msg.ID, Code: 1, Comment: err.Error()}, false)
	}
	dev.Mu.Lock()
	err = SendDevice(msg.ID, dev, true, c)
	dev.Mu.Unlock()
	if err != nil {
		return err
	}
	return c.sendOutgoingEventMessage("board-profile-result", DeviceCommentCodeMessage{ID: msg.ID, Code: 0}, false)
}

// Keep the detected identity separately so automatic detection can be restored.
func (d *Detector) withBoardProfiles(detectedBoards map[string]*Device) map[string]*Device {
	d.detectedBoards = make(map[string]*Device, len(detectedBoards))
	for id, dev := range detectedBoards {
		d.detectedBoards[id] = newDevice(*dev.TypeDesc, dev.Board)
		if profileID, ok := d.manualProfiles[id]; ok {
			if profile := d.manualProfile(profileID); profile != nil {
				detectedBoards[id] = deviceWithProfile(dev, *profile)
			}
		}
	}
	return detectedBoards
}
