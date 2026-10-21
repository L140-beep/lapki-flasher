package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	monitorSerial "github.com/albenik/go-serial/v2"
	"go.bug.st/serial/enumerator"
)

func testProfile(id int, name, controller, programmer string, bootloader int) BoardTemplate {
	payload, _ := json.Marshal(ArduinoPayload{Controller: controller, Programmer: programmer, BootloaderID: bootloader})
	return BoardTemplate{ID: id, Name: name, Type: "arduino", TypePayload: payload, FlashFileExtension: "hex"}
}

func testDetector() *Detector {
	templates := []BoardTemplate{
		testProfile(10, "Arduino Uno", "ATmega328P", "arduino", -1),
		testProfile(20, "Arduino Micro", "ATmega32U4", "avr109", 21),
		testProfile(21, "Arduino Micro (bootloader)", "ATmega32U4", "avr109", -1),
	}
	source := newDevice(BoardTemplate{ID: -1, Name: "Неизвестная плата", Type: "unknown"}, &UnknownPort{portName: "COM5", serialID: "clone"})
	return &Detector{
		boardTemplates: templates,
		boards:         map[string]*Device{"serial:COM5": newDevice(*source.TypeDesc, source.Board)},
		detectedBoards: map[string]*Device{"serial:COM5": source},
		manualProfiles: make(map[string]int),
	}
}

func TestUnknownPortsExcludeEveryRecognizedPort(t *testing.T) {
	uno := testProfile(10, "Arduino Uno", "ATmega328P", "arduino", -1)
	boards := map[string]*Device{
		"uno": newDevice(uno, NewArduinoFromTemp(uno, "COM1", ArduinoOS{}, "")),
		"ms":  newDevice(BoardTemplate{Type: "tjc-ms"}, NewMS1([4]string{"COM2", "COM3", "COM4", "COM5"}, MS1OS{})),
	}
	addUnknownPorts(boards, []string{"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM6", ""}, map[string]*enumerator.PortDetails{
		"COM6": {Name: "COM6", VID: "1A86", PID: "7523", SerialNumber: "clone"},
	})
	if len(boards) != 3 {
		t.Fatalf("expected only one unknown port: %v", boards)
	}
	unknown := boards["serial:COM6"].Board.(*UnknownPort)
	if unknown.vendorID != "1A86" || unknown.serialID != "clone" {
		t.Fatal("missing metadata")
	}
	if _, err := unknown.Flash("firmware.hex", nil); err != ErrNotSupported {
		t.Fatal("unclassified ports must not flash")
	}
}

func TestProfilesExcludeBootloaderIdentities(t *testing.T) {
	d := testDetector()
	profiles := d.boardProfiles()
	if len(profiles) != 2 || profiles[0].ID != 10 || profiles[1].ID != 20 {
		t.Fatalf("unexpected profiles: %v", profiles)
	}
	if d.manualProfile(-1) != nil || d.manualProfile(21) != nil || d.manualProfile(100) != nil {
		t.Fatal("invalid profiles accepted")
	}
}

func TestAssignUnknownPortRefreshAndReset(t *testing.T) {
	d := testDetector()
	profileID := 10
	dev, err := d.setBoardProfile("serial:COM5", &profileID)
	if err != nil {
		t.Fatal(err)
	}
	arduino := dev.Board.(*Arduino)
	if !arduino.manualPort || arduino.controller != "ATmega328P" || arduino.portName != "COM5" || !arduino.manuallyAssigned {
		t.Fatalf("wrong board: %+v", arduino)
	}
	refreshed := d.withBoardProfiles(map[string]*Device{
		"serial:COM5": newDevice(*d.detectedBoards["serial:COM5"].TypeDesc, &UnknownPort{portName: "COM5"}),
		"serial:COM6": newDevice(BoardTemplate{ID: -1, Type: "unknown"}, &UnknownPort{portName: "COM6"}),
	})
	if _, ok := refreshed["serial:COM5"].Board.(*Arduino); !ok {
		t.Fatal("assignment lost on refresh")
	}
	if _, ok := refreshed["serial:COM6"].Board.(*UnknownPort); !ok {
		t.Fatal("assignment leaked into another port")
	}
	reset, err := d.setBoardProfile("serial:COM5", nil)
	if err != nil {
		t.Fatal(err)
	}
	if reset != dev {
		t.Fatal("device identity must stay stable")
	}
	if _, ok := reset.Board.(*UnknownPort); !ok {
		t.Fatal("automatic detection did not restore unknown port")
	}
	if len(d.manualProfiles) != 0 {
		t.Fatal("manual assignment not cleared")
	}
}

func TestRecognizedArduinoCanBeCorrected(t *testing.T) {
	d := testDetector()
	original := NewArduinoFromTemp(d.boardTemplates[1], "COM7", ArduinoOS{}, "micro")
	source := newDevice(d.boardTemplates[1], original)
	d.detectedBoards["recognized"] = source
	d.boards["recognized"] = newDevice(*source.TypeDesc, source.Board)
	profileID := 10
	dev, err := d.setBoardProfile("recognized", &profileID)
	if err != nil {
		t.Fatal(err)
	}
	if dev.Board.(*Arduino).controller != "ATmega328P" {
		t.Fatal("profile was not changed")
	}
	dev, err = d.setBoardProfile("recognized", nil)
	if err != nil || dev.Board.(*Arduino).controller != "ATmega32U4" {
		t.Fatal("automatic profile was not restored", err)
	}
}

func TestRejectProfileChangesForBusyAndMissingDevices(t *testing.T) {
	d := testDetector()
	id := 10
	dev := d.boards["serial:COM5"]
	dev.Flashing = true
	if _, err := d.setBoardProfile("serial:COM5", &id); err == nil {
		t.Fatal("changed profile while flashing")
	}
	dev.Flashing = false
	dev.SerialMonitor.Open = true
	// isOpen also checks the port pointer; tested using a closed zero-value handle.
	dev.SerialMonitor.Port = &monitorSerial.Port{}
	if _, err := d.setBoardProfile("serial:COM5", &id); err == nil {
		t.Fatal("changed profile with open monitor")
	}
	dev.SerialMonitor.Port = nil
	if _, err := d.setBoardProfile("missing", &id); err == nil {
		t.Fatal("accepted absent device")
	}
	invalid := -1
	if _, err := d.setBoardProfile("serial:COM5", &invalid); err == nil {
		t.Fatal("accepted invalid profile")
	}
	d.DeleteBoard("serial:COM5")
	if len(d.manualProfiles) != 0 || len(d.detectedBoards) != 0 {
		t.Fatal("disconnected port still has assignments")
	}
}

func TestProfileProtocolBroadcastsDeviceBeforeAcknowledgement(t *testing.T) {
	previous := detector
	detector = testDetector()
	defer func() { detector = previous }()
	c := &WebSocketConnection{outgoingMsg: make(chan OutgoingEventMessage, 4)}
	if err := SetBoardProfile(Event{Payload: json.RawMessage(`{"deviceID":"serial:COM5","profileID":10}`)}, c); err != nil {
		t.Fatal(err)
	}
	device := <-c.outgoingMsg
	ack := <-c.outgoingMsg
	if device.event.Type != "device" || !device.toAll || ack.event.Type != "board-profile-result" {
		t.Fatal("unexpected protocol sequence")
	}
	var message DeviceMessage
	if err := json.Unmarshal(device.event.Payload, &message); err != nil {
		t.Fatal(err)
	}
	if message.ProfileID != 10 || !message.ManuallyAssigned || message.PortName != "COM5" {
		t.Fatalf("wrong profile message: %+v", message)
	}
	if err := SetBoardProfile(Event{Payload: json.RawMessage(`{"deviceID":"serial:COM5","profileID":-1}`)}, c); err != nil {
		t.Fatal(err)
	}
	var result DeviceCommentCodeMessage
	json.Unmarshal((<-c.outgoingMsg).event.Payload, &result)
	if result.Code == 0 {
		t.Fatal("expected explicit assignment error")
	}
}

func TestManualUnoUsesSelectedAvrdudeProfile(t *testing.T) {
	// A tiny fake avrdude checks the real subprocess arguments without touching hardware.
	if os.PathSeparator == '\\' {
		t.Skip("shell script requires Unix")
	}
	d := testDetector()
	id := 10
	dev, err := d.setBoardProfile("serial:COM5", &id)
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "avrdude")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	oldPath, oldConfig := avrdudePath, configPath
	avrdudePath, configPath = script, ""
	defer func() { avrdudePath, configPath = oldPath, oldConfig }()
	output, err := dev.Board.Flash("firmware.hex", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"-p\nATmega328P", "-c\narduino", "-P\nCOM5", "flash:w:"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("missing %q in avrdude arguments: %s", expected, output)
		}
	}
}

func TestMacOSPortAliasesAreNotDuplicated(t *testing.T) {
	boards := make(map[string]*Device)
	addUnknownPorts(boards, []string{"/dev/cu.usbserial", "/dev/tty.usbserial"}, nil)
	if len(boards) != 1 {
		t.Fatal("macOS aliases were duplicated")
	}
	uno := testProfile(10, "Arduino Uno", "ATmega328P", "arduino", -1)
	boards = map[string]*Device{"uno": newDevice(uno, NewArduinoFromTemp(uno, "/dev/tty.usbserial", ArduinoOS{}, ""))}
	addUnknownPorts(boards, []string{"/dev/cu.usbserial", "/dev/tty.usbserial"}, nil)
	if len(boards) != 1 {
		t.Fatal("recognized port alias became unknown")
	}
}

func TestUnknownPortFlashStartIsRejectedBeforeUpload(t *testing.T) {
	previous, previousLimit := detector, maxFileSize
	detector, maxFileSize = testDetector(), 1024
	defer func() { detector, maxFileSize = previous, previousLimit }()
	c := &WebSocketConnection{}
	err := FlashStart(Event{Type: FlashStartMsg, Payload: json.RawMessage(`{"deviceID":"serial:COM5","fileSize":10}`)}, c)
	if err != ErrNotSupported || c.FlashingBoard != nil {
		t.Fatal("unknown port accepted for upload", err)
	}
}
