//go:build linux

package main

import (
	"fmt"
	"io/fs"
	stdlog "log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/google/gousb"
)

type ArduinoOS struct {
	deviceID  string
	productID string
	vendorID  string
}

type MS1OS struct {
	deviceID string
}

const DEV = "/dev"
const ID_SERIAL = "ID_SERIAL_SHORT"
const USEC_INITIALIZED = "USEC_INITIALIZED"

// настройка для выбранной ОС
func setupOS() {
	stdlog.SetOutput(new(LogrusWriter))

}

// удаление сообщения "interrupted [code -10]" из консоли

type LogrusWriter int

const interruptedError = "interrupted [code -10]"

func (LogrusWriter) Write(data []byte) (int, error) {
	logmessage := string(data)
	if strings.Contains(logmessage, interruptedError) {
		log.Tracef("gousb_logs:%s", logmessage)
		return len(data), nil
	}
	log.Infof("gousb_logs:%s", logmessage)
	return len(data), nil
}

// находит все подключённые платы
func detectBoards(boardTemplates []BoardTemplate) map[string]*Device {
	ctx := gousb.NewContext()
	defer ctx.Close()

	devs := make(map[string]*Device)

	_, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		for _, boardTemplate := range boardTemplates {
			vid := ""
			pid := ""
			foundVidPid := false
			for _, pidvid := range boardTemplate.PidVid {
				vid = pidvid.VendorID
				pid = pidvid.ProductID
				if strings.ToLower(desc.Vendor.String()) == strings.ToLower(vid) && strings.ToLower(pid) == strings.ToLower(desc.Product.String()) {
					foundVidPid = true
					break
				}
			}
			if !foundVidPid {
				continue
			}
			if boardTemplate.IsBlgMbDevice() {
				devs[pid+vid+"blg-mb"] = newDevice(boardTemplate, &BlgMb{})
				break
			}
			ports := findPortName(desc)
			portsNum := len(ports)
			if portsNum < 1 || ports[0] == NOT_FOUND {
				printLog("can't find port", ports)
				break
			}
			properties, err := findProperty(ports[0], USEC_INITIALIZED, ID_SERIAL)
			if err != nil {
				printLog("can't find ID", err.Error())
				break
			}
			serialID := properties[1]
			var id string
			// на данный момент у всех МС-ТЮК одинаковый serialID, поэтому мы его игнорируем
			if serialID != NOT_FOUND && !boardTemplate.IsMSDevice() {
				id = serialID
			} else {
				id = properties[0]
			}
			var board Board
			if boardTemplate.IsMSDevice() {
				if portsNum != 4 {
					printLog("Number of ports for ms1 should be equal to 4. Number of ports for this device:", portsNum)
					break
				}
				board = NewMS1(
					[4]string{
						ports[0],
						ports[1],
						ports[2],
						ports[3],
					},
					MS1OS{
						deviceID: id,
					},
				)
			} else if boardTemplate.IsArduinoDevice() {
				board = NewArduinoFromTemp(
					boardTemplate,
					ports[0],
					ArduinoOS{
						deviceID:  id,
						productID: pid,
						vendorID:  vid,
					},
					serialID,
				)
			} else {
				printLog("no searching algorithm for this type of device!", boardTemplate.Type)
				break
			}
			devs[id] = newDevice(boardTemplate, board)
			break
		}
		return false
	})

	if err != nil {
		log.Printf("OpenDevices(): %v\n", err)
		return nil
	}

	return devs
}

func hasFound(ID string, isSerial bool, portName string) bool {
	var properties []string
	var err error
	if isSerial {
		properties, err = findProperty(portName, ID_SERIAL)
	} else {
		properties, err = findProperty(portName, USEC_INITIALIZED)
	}
	if err == nil && properties[0] == ID {
		return true
	}
	return false
}

/*
Возвращает порты устройства, возвращает nil, если ничего не удалось найти.

Для arduino-подобных устройств должен вернуть один порт, для МС-ТЮК - четыре порта.
*/
func findPortName(desc *gousb.DeviceDesc) []string {
	// <bus>-<port[.port[.port]]>:<config>.<interface> - шаблон папки в которой должен находиться путь к папке tty

	ports := strconv.Itoa(desc.Path[0])
	num_ports := len(desc.Path)
	for i := 1; i < num_ports; i++ {
		ports += "." + strconv.Itoa(desc.Path[i])
	}

	// рекурсивно проходимся по возможным config и interface до тех пор пока не найдём tty папку
	dir_prefix := "/sys/bus/usb/devices"
	tty := "tty"
	var portNames []string
	for _, conf := range desc.Configs {
		for _, inter := range conf.Interfaces {
			root := fmt.Sprintf("%s/%d-%s:%d.%d", dir_prefix, desc.Bus, ports, conf.Number, inter.Number)
			fileSystem := os.DirFS(root)
			var devicePath string
			fs.WalkDir(fileSystem, ".", func(path string, d fs.DirEntry, err error) error {
				if d.Name() == tty {
					// использование Readdirnames вместо ReadDir может ускорить работу в 20 раз
					dirs, err := os.ReadDir(root + "/" + path)
					if err != nil {
						return err
					}
					if len(dirs) < 1 {
						return nil
					}
					devicePath = fmt.Sprintf("%s/%s", DEV, dirs[0].Name())
					return fs.SkipAll
				}
				return nil
			})
			if devicePath != NOT_FOUND {
				portNames = append(portNames, devicePath)
			}
		}
	}
	return portNames
}

// возвращает значение указанных параметров устройства, подключённого к порту portName,
// можно использовать для того, чтобы получить серийный номер устройства (если есть) или для получения времени, когда устройство было подключено (используется как ID)
//
//	см. "udevadm info --query=propery" для большей информации об параметрах
func findProperty(portName string, properties ...string) ([]string, error) {
	numProperties := len(properties)
	if numProperties == 0 {
		return nil, nil
	}
	cmd := exec.Command("udevadm", "info", "--query=property", "--name="+portName)
	stdout, err := cmd.CombinedOutput()
	if err != nil {
		printLog(string(stdout), err.Error())
		return nil, err
	}
	lines := strings.Split(string(stdout), "\n")
	var answers = make([]string, numProperties)
	for _, line := range lines {
		lineSize := len(line)
		for i, property := range properties {
			if answers[i] != "" {
				continue
			}
			propertySize := len(property)
			if propertySize > lineSize {
				continue
			}
			if line[:propertySize] == property {
				answers[i] = line[propertySize+1:]
			}
		}
	}
	return answers, nil
}

// перезагрузка порта
func rebootPort(portName string) (err error) {
	// stty 115200 -F /dev/ttyUSB0 raw -echo
	cmd := exec.Command("stty", "1200", "-F", portName, "raw", "-echo")
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
	if hasFound(board.ardOS.deviceID, board.hasSerial(), board.portName) {
		return false
	}
	board.portName = NOT_FOUND
	if !board.hasSerial() {
		return true
	}
	ctx := gousb.NewContext()
	defer ctx.Close()
	var portName string
	ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		if desc.Product.String() == board.ardOS.productID && desc.Vendor.String() == board.ardOS.vendorID {
			portNames := findPortName(desc)
			portsNum := len(portNames)
			if portsNum == 0 || portsNum > 1 {
				printLog("number of ports is not equal to 1", portsNum)
			} else {
				properties, _ := findProperty(portNames[0], ID_SERIAL)
				printLog("prop", properties)
				if properties[0] == board.serialID {
					portName = portNames[0]
					return false
				}
			}
		}
		return false
	})
	board.portName = portName
	return true
}

func (board *MS1) Update() bool {
	if !hasFound(board.ms1OS.deviceID, false, board.portNames[0]) {
		board.portNames[0] = NOT_FOUND
		return true
	}
	return false
}
