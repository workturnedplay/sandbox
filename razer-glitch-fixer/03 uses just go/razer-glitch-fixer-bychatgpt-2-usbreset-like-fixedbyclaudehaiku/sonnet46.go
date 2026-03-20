package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	setupapi                         = windows.NewLazySystemDLL("setupapi.dll")
	procSetupDiGetClassDevs          = setupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInfo        = setupapi.NewProc("SetupDiEnumDeviceInfo")
	procSetupDiGetDeviceInstanceId   = setupapi.NewProc("SetupDiGetDeviceInstanceIdW")
	procSetupDiGetDeviceRegistryProp = setupapi.NewProc("SetupDiGetDeviceRegistryPropertyW")
	procSetupDiDestroyDeviceInfoList = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
	procSetupDiSetClassInstallParams = setupapi.NewProc("SetupDiSetClassInstallParamsW")
	procSetupDiCallClassInstaller    = setupapi.NewProc("SetupDiCallClassInstaller")
	procSetupDiGetDeviceInstallParams = setupapi.NewProc("SetupDiGetDeviceInstallParamsW")
)

const (
	DIGCF_ALLCLASSES = 0x00000004
	DIGCF_PRESENT    = 0x00000002
	SPDRP_DEVICEDESC = 0x00000000

	DIF_PROPERTYCHANGE = 0x00000012

	DICS_PROPCHANGE      = 0x00000003
	DICS_FLAG_CONFIGSPECIFIC = 0x00000002

	DI_NEEDRESTART = 0x00000001
	DI_NEEDREBOOT  = 0x00000010

	// ERROR_DI_DO_DEFAULT is returned when there is no class installer but the
	// default action should be performed — this is non-fatal and expected.
	ERROR_DI_DO_DEFAULT = windows.Errno(0xE000020E)
)

type SP_DEVINFO_DATA struct {
	CbSize    uint32
	ClassGuid windows.GUID
	DevInst   uint32
	Reserved  uintptr
}

type SP_CLASSINSTALL_HEADER struct {
	CbSize          uint32
	InstallFunction uint32
}

type SP_PROPCHANGE_PARAMS struct {
	ClassInstallHeader SP_CLASSINSTALL_HEADER
	StateChange        uint32
	Scope              uint32
	HwProfile          uint32
}

type SP_DEVINSTALL_PARAMS struct {
	CbSize                  uint32
	Flags                   uint32
	FlagsEx                 uint32
	HwndParent              uintptr
	InstallMsgHandler       uintptr
	InstallMsgContext       uintptr
	FileQueue               uintptr
	ClassInstallReserved    uintptr
	Reserved                uint32
	DriverPath              [260]uint16
}

type Device struct {
	InstanceID string
	Name       string
	devData    SP_DEVINFO_DATA
}

func main() {
	vid := "1532"
	pid := "0109"

	// Allow overriding VID/PID from command line: program.exe [VID] [PID]
	args := os.Args[1:]
	switch len(args) {
	case 2:
		vid = args[0]
		pid = args[1]
	case 1:
		log.Fatal("Usage: program [VID PID] — provide both VID and PID or neither")
	}

	h, devices, err := findDevices(vid, pid)
	if err != nil {
		log.Fatalf("findDevices: %v", err)
	}
	defer procSetupDiDestroyDeviceInfoList.Call(h)

	if len(devices) == 0 {
		log.Fatalf("no devices found for VID_%s&PID_%s", vid, pid)
	}

	// We want the USB composite device node — no MI_ in the instance ID.
	// MI_ nodes are interface children and won't produce a full power cycle.
	var target *Device
	for i := range devices {
		if !strings.Contains(devices[i].InstanceID, "MI_") {
			target = &devices[i]
			break
		}
	}
	if target == nil {
		log.Fatalf("found %d device(s) for VID_%s&PID_%s but all are MI_ interface nodes", len(devices), vid, pid)
	}

	fmt.Printf("Restarting: %s (%s)\n", target.InstanceID, target.Name)

	if err := restartDevice(h, target); err != nil {
		log.Fatalf("restart failed: %v", err)
	}

	fmt.Println("Done.")
}

// findDevices enumerates all present devices and returns those matching VID/PID.
// The caller is responsible for calling SetupDiDestroyDeviceInfoList on the
// returned handle when done.
func findDevices(vid, pid string) (uintptr, []Device, error) {
	h, _, err := procSetupDiGetClassDevs.Call(
		0, 0, 0,
		DIGCF_ALLCLASSES|DIGCF_PRESENT,
	)
	// SetupDiGetClassDevs returns INVALID_HANDLE_VALUE (^uintptr(0)) on failure,
	// not 0 — but checking both is safe.
	if h == 0 || h == ^uintptr(0) {
		return 0, nil, fmt.Errorf("SetupDiGetClassDevs failed: %w", err)
	}

	needle := fmt.Sprintf("VID_%s&PID_%s", vid, pid)

	var devices []Device
	for i := 0; ; i++ {
		var data SP_DEVINFO_DATA
		data.CbSize = uint32(unsafe.Sizeof(data))

		r1, _, _ := procSetupDiEnumDeviceInfo.Call(
			h,
			uintptr(i),
			uintptr(unsafe.Pointer(&data)),
		)
		if r1 == 0 {
			// ERROR_NO_MORE_ITEMS — normal loop termination
			break
		}

		instanceID, err := getInstanceID(h, &data)
		if err != nil {
			// Non-fatal: skip devices whose instance ID we can't read
			continue
		}

		if !strings.Contains(instanceID, needle) {
			continue
		}

		name, err := getDeviceDesc(h, &data)
		if err != nil {
			// Non-fatal: name is cosmetic, proceed without it
			name = "(unknown)"
		}

fmt.Printf("DEBUG found device: %s\n", instanceID)
		devices = append(devices, Device{
			InstanceID: instanceID,
			Name:       name,
			devData:    data,
		})
	}

	return h, devices, nil
}

func restartDevice(h uintptr, d *Device) error {
	pcp := SP_PROPCHANGE_PARAMS{
		ClassInstallHeader: SP_CLASSINSTALL_HEADER{
			CbSize:          uint32(unsafe.Sizeof(SP_CLASSINSTALL_HEADER{})),
			InstallFunction: DIF_PROPERTYCHANGE,
		},
		StateChange: DICS_PROPCHANGE,
		Scope:       DICS_FLAG_CONFIGSPECIFIC,
		HwProfile:   0,
	}

	r1, _, err := procSetupDiSetClassInstallParams.Call(
		h,
		uintptr(unsafe.Pointer(&d.devData)),
		uintptr(unsafe.Pointer(&pcp)),
		uintptr(unsafe.Sizeof(pcp)),
	)
	if r1 == 0 {
		return fmt.Errorf("SetupDiSetClassInstallParams failed: %w", err)
	}

	r1, _, err = procSetupDiCallClassInstaller.Call(
		DIF_PROPERTYCHANGE,
		h,
		uintptr(unsafe.Pointer(&d.devData)),
	)
  fmt.Printf("DEBUG CallClassInstaller: r1=%d err=%v (0x%X)\n", r1, err, uintptr(err.(windows.Errno)))
	if r1 == 0 {
		// ERROR_DI_DO_DEFAULT means no class installer is registered but the
		// default action (the actual restart) will still be performed — not an error.
		if !errors.Is(err, ERROR_DI_DO_DEFAULT) {
			return fmt.Errorf("SetupDiCallClassInstaller failed: %w", err)
		}
	}

	// Check whether the OS flagged a reboot as required — informational only.
	var devParams SP_DEVINSTALL_PARAMS
	devParams.CbSize = uint32(unsafe.Sizeof(devParams))
	r1, _, err = procSetupDiGetDeviceInstallParams.Call(
		h,
		uintptr(unsafe.Pointer(&d.devData)),
		uintptr(unsafe.Pointer(&devParams)),
	)
	if r1 == 0 {
		// Non-fatal — we still issued the restart, just can't check reboot flag
		fmt.Fprintf(os.Stderr, "warning: SetupDiGetDeviceInstallParams failed: %v\n", err)
	} else if devParams.Flags&DI_NEEDRESTART != 0 || devParams.Flags&DI_NEEDREBOOT != 0 {
		fmt.Println("Note: a reboot may be required for changes to take full effect.")
	}

	return nil
}

func getInstanceID(h uintptr, data *SP_DEVINFO_DATA) (string, error) {
	buf := make([]uint16, 512)
	var required uint32
	r1, _, err := procSetupDiGetDeviceInstanceId.Call(
		h,
		uintptr(unsafe.Pointer(data)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&required)),
	)
	if r1 == 0 {
		return "", fmt.Errorf("SetupDiGetDeviceInstanceId failed: %w", err)
	}
	return windows.UTF16ToString(buf), nil
}

func getDeviceDesc(h uintptr, data *SP_DEVINFO_DATA) (string, error) {
	// Allocate as uint16 directly — avoids the unsafe byte->uint16 cast
	buf := make([]uint16, 256)
	var regType, required uint32
	r1, _, err := procSetupDiGetDeviceRegistryProp.Call(
		h,
		uintptr(unsafe.Pointer(data)),
		SPDRP_DEVICEDESC,
		uintptr(unsafe.Pointer(&regType)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)*2), // size in bytes
		uintptr(unsafe.Pointer(&required)),
	)
	if r1 == 0 {
		return "", fmt.Errorf("SetupDiGetDeviceRegistryProperty failed: %w", err)
	}
	return windows.UTF16ToString(buf), nil
}