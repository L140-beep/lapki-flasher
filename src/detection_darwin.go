//go:build darwin

package main

import (
	"log"
	"os/exec"
	"sort"
	"strconv"

	"howett.net/plist"
)

type ArduinoOS struct {
	ID string
}

type MS1OS struct {
}

type IOREG struct {
	VendorID  int64 `plist:"idVendor"`
	ProductID int64 `plist:"idProduct"`
	// Время с момента последнего запуска ОС.
	// Используется для идентификации устройства, в случае отсутствия серийного номера
	SessionID    int64   `plist:"sessionID"`
	SerialNumber string  `plist:"USB Serial Number"`
	Port         string  `plist:"IODialinDevice"`
	Children     []IOREG `plist:"IORegistryEntryChildren"`
}

// настройка ОС (для Darwin она не требуется, но она здесь присутствует, чтобы обеспечить совместимость с другими платформами, которые используют свои реализации этой функции)
func setupOS() {}

// находит все подключённые платы
// TODO: добавить поиск сериного номера
func detectBoards(boardTemplates []BoardTemplate) map[string]*Device {
	devices := make(map[string]*Device)
	cmd := exec.Command("ioreg", "-r", "-c", "IOUSBHostDevice", "-l", "-a")
	plistData, err := cmd.CombinedOutput()
	if err != nil {
		printLog("plist error", string(plistData), err.Error())
		return nil
	}
	plistArr := []IOREG{}
	format, err := plist.Unmarshal(plistData, &plistArr)
	if err != nil {
		printLog("unmarshal error:", err.Error(), cmd.String())
		printLog("plint format:", format)
		//printLog(string(plistData))
		return nil
	}
	IOREGscan(plistArr, boardTemplates, devices)
	return devices
}

func IOREGport(plistArr []IOREG, ID string, board *Arduino) (portName string, foundID bool) {
	for _, entry := range plistArr {
		if (entry.SerialNumber == "" && strconv.FormatInt(entry.SessionID, 10) == ID) || entry.SerialNumber == ID {
			detectedBoard := CopyArduino(board)
			detectedBoard.portName = NOT_FOUND
			collectArduinoBoardInfo(entry, detectedBoard)
			if detectedBoard.portName == NOT_FOUND {
				printLog("can't find port name!")
				detectedBoard.portName = NOT_FOUND
				return NOT_FOUND, true
			}
			return detectedBoard.portName, true
		}
	}
	for _, entry := range plistArr {
		port, found := IOREGport(entry.Children, ID, board)
		if found {
			return port, found
		}
	}
	return NOT_FOUND, false
}

// перезагрузка порта
func rebootPort(portName string) (err error) {
	// stty 115200 -F /dev/ttyUSB0 raw -echo
	cmd := exec.Command("stty", "-f", portName, "1200")
	_, err = cmd.CombinedOutput()
	printLog(cmd.Args)
	if err != nil {
		printLog(cmd.Args, err)
	}
	return err
}

func IOREGscan(plistArr []IOREG, boardTemplates []BoardTemplate, boards map[string]*Device) {
	for _, entry := range plistArr {
		isFound := false
		for _, boardTemplate := range boardTemplates {
			for _, pidvid := range boardTemplate.PidVid {
				productID := pidvid.ProductID
				PID, err := strconv.ParseInt(productID, 16, 64)
				if err != nil {
					printLog("conv of product id to decimal is failed: ", err.Error())
					continue
				}
				vendorID := pidvid.VendorID
				VID, err := strconv.ParseInt(vendorID, 16, 64)
				if err != nil {
					printLog("conv of vendor id to decimal is failed: ", err.Error())
					continue
				}
				if entry.ProductID == PID && entry.VendorID == VID {
					var board Board
					var ID string
					if boardTemplate.IsMSDevice() {
						// TODO
						portsMap := make(map[string]struct{}, 4)
						ID = strconv.FormatInt(collectMSBoardInfo(entry, portsMap), 10)
						if ID == "" || ID == "0" {
							printLog("can't find ID!")
							goto SKIP
						}
						portsLen := len(portsMap)
						if portsLen != 4 {
							log.Println("Error: incorrect number of ports for ms1 device:", portsLen)
							goto SKIP
						}
						ports := make([]string, portsLen)
						i := 0
						for port, _ := range portsMap {
							ports[i] = port
							i++
						}
						sort.Slice(ports, func(i, j int) bool {
							len_1 := len(ports[i])
							len_2 := len(ports[j])
							if len_1 != len_2 {
								return len_1 < len_2
							} else {
								return ports[i] < ports[j]
							}
						})
						board = NewMS1(
							[4]string{ports[0], ports[1], ports[2], ports[3]},
							MS1OS{},
						)
					} else if boardTemplate.IsArduinoDevice() {
						arduino := NewArduinoFromTemp(
							boardTemplate,
							NOT_FOUND,
							ArduinoOS{},
							NOT_FOUND,
						)
						ID = strconv.FormatInt(collectArduinoBoardInfo(entry, arduino), 10)
						if arduino.serialID != "" {
							ID = arduino.serialID
						}
						if ID == "" || ID == "0" {
							printLog("can't find ID!")
							goto SKIP
						}
						if arduino.portName == NOT_FOUND {
							printLog("can't find port name!")
							goto SKIP
						}
						arduino.ardOS.ID = ID
						board = arduino
					} else if boardTemplate.IsBlgMbDevice() {
						ID = productID + vendorID + "blg-mb"
						board = &BlgMb{}
					} else {
						printLog("no searching algorithm for this type of device!", boardTemplate.Type)
						continue
					}
					detectedDevice := newDevice(
						boardTemplate,
						board,
					)
					boards[ID] = detectedDevice
					printLog("Found device", ID, detectedDevice)
					isFound = true
					goto SKIP

				}

			}
		}
	SKIP:
		if !isFound {
			IOREGscan(entry.Children, boardTemplates, boards)
		}
	}
}

func collectArduinoBoardInfo(reg IOREG, board *Arduino) (sessionID int64) {
	if reg.SerialNumber != "" {
		board.serialID = reg.SerialNumber
	}
	if reg.Port != "" {
		board.portName = reg.Port
	}
	if reg.SessionID != 0 {
		sessionID = reg.SessionID
	}
	for _, child := range reg.Children {
		res := collectArduinoBoardInfo(child, board)
		if res != 0 {
			sessionID = res
		}
	}
	return sessionID
}

func collectMSBoardInfo(reg IOREG, ports map[string]struct{}) (sessionID int64) {
	if reg.Port != "" {
		ports[reg.Port] = struct{}{}
	}
	if reg.SessionID != 0 {
		sessionID = reg.SessionID
	}
	for _, child := range reg.Children {
		res := collectMSBoardInfo(child, ports)
		if res != 0 {
			sessionID = res
		}
	}
	return sessionID
}

func (board *Arduino) Update() bool {
	if board.manualPort {
		return updateManualPort(&board.portName)
	}
	cmd := exec.Command("ioreg", "-r", "-c", "IOUSBHostDevice", "-l", "-a")
	plistData, err := cmd.CombinedOutput()
	if err != nil {
		printLog("plist error", string(plistData), err.Error())
		board.portName = NOT_FOUND
		return true
	}
	plistArr := []IOREG{}
	format, err := plist.Unmarshal(plistData, &plistArr)
	if err != nil {
		printLog("unmarshal error:", err.Error(), cmd.String())
		printLog("plint format:", format)
		//printLog(string(plistData))
		board.portName = NOT_FOUND
		return true
	}
	portName, _ := IOREGport(plistArr, board.ardOS.ID, board)
	if portName == NOT_FOUND {
		board.portName = NOT_FOUND
		return true
	}
	if portName != board.portName {
		board.portName = portName
		return true
	}
	return false
}

func (board *MS1) Update() bool {
	return false
}
