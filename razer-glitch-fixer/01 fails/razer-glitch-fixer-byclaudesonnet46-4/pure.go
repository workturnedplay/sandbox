package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	setupapi                          = windows.NewLazySystemDLL("setupapi.dll")
	procSetupDiGetClassDevs           = setupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInfo         = setupapi.NewProc("SetupDiEnumDeviceInfo")
	procSetupDiGetDeviceInstanceId    = setupapi.NewProc("SetupDiGetDeviceInstanceIdW")
	procSetupDiGetDeviceRegistryProp  = setupapi.NewProc("SetupDiGetDeviceRegistryPropertyW")
	procSetupDiDestroyDeviceInfoList  = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
	procSetupDiSetClassInstallParams  = setupapi.NewProc("SetupDiSetClassInstallParamsW")
	procSetupDiCallClassInstaller     = setupapi.NewProc("SetupDiCallClassInstaller")
)

const (
	DIGCF_ALLCLASSES = 0x00000004
	DIGCF_PRESENT    = 0x00000002
	SPDRP_DEVICEDESC = 0x00000000

	DIF_PROPERTYCHANGE = 0x00000012

	DICS_ENABLE  = 0x00000001
	DICS_DISABLE = 0x00000002

	DICS_FLAG_GLOBAL = 0x00000001
)

type SP_DEVINFO_DATA struct {
	CbSize    uint32
	ClassGuid windows.GUID
	DevInst   uint32
	Reserved  uintptr
}

// SP_PROPCHANGE_PARAMS holds DIF_PROPERTYCHANGE parameters.
// It must be prefixed with SP_CLASSINSTALL_HEADER.
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

type Device struct {
	InstanceID string
	Name       string
	devInfo    uintptr
	devData    SP_DEVINFO_DATA
}

func main() {
	const vid = "1532"
	const pid = "0109"

	h, devices, err := findDevices(vid, pid)
	if err != nil {
		log.Fatal(err)
	}
	defer procSetupDiDestroyDeviceInfoList.Call(h)

	if len(devices) == 0 {
		fmt.Fprintf(os.Stderr, "No devices found for VID_%s&PID_%s\n", vid, pid)
		os.Exit(1)
	}

	// Filter to the USB composite device (no MI_)
	var target *Device
	for i := range devices {
		if !strings.Contains(devices[i].InstanceID, "MI_") {
			target = &devices[i]
			break
		}
	}
	if target == nil {
		fmt.Fprintf(os.Stderr, "No composite USB device found (all results contain MI_)\n")
		os.Exit(1)
	}

	fmt.Printf("Restarting: %s (%s)\n", target.InstanceID, target.Name)

	if err := restartDevice(h, target); err != nil {
		log.Fatalf("restart failed: %v", err)
	}

	fmt.Println("Done.")
}

func findDevices(vid, pid string) (uintptr, []Device, error) {
	h, _, err := procSetupDiGetClassDevs.Call(
		0, 0, 0,
		DIGCF_ALLCLASSES|DIGCF_PRESENT,
	)
	if h == 0 {
		return 0, nil, fmt.Errorf("SetupDiGetClassDevs failed: %w", err)
	}

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
			break
		}

		instanceID, err := getInstanceID(h, &data)
		if err != nil {
			continue
		}

		needle := fmt.Sprintf("VID_%s&PID_%s", vid, pid)
		if !strings.Contains(instanceID, needle) {
			continue
		}

		name, _ := getDeviceDesc(h, &data)
		devices = append(devices, Device{
			InstanceID: instanceID,
			Name:       name,
			devInfo:    h,
			devData:    data,
		})
	}

	return h, devices, nil
}

func restartDevice(h uintptr, d *Device) error {
	if err := propChange(h, d, DICS_DISABLE); err != nil {
		return fmt.Errorf("disable failed: %w", err)
	}
	if err := propChange(h, d, DICS_ENABLE); err != nil {
		return fmt.Errorf("enable failed: %w", err)
	}
	return nil
}

func propChange(h uintptr, d *Device, stateChange uint32) error {
	params := SP_PROPCHANGE_PARAMS{
		ClassInstallHeader: SP_CLASSINSTALL_HEADER{
			CbSize:          uint32(unsafe.Sizeof(SP_CLASSINSTALL_HEADER{})),
			InstallFunction: DIF_PROPERTYCHANGE,
		},
		StateChange: stateChange,
		Scope:       DICS_FLAG_GLOBAL,
		HwProfile:   0,
	}

	r1, _, err := procSetupDiSetClassInstallParams.Call(
		h,
		uintptr(unsafe.Pointer(&d.devData)),
		uintptr(unsafe.Pointer(&params)),
		uintptr(unsafe.Sizeof(params)),
	)
	if r1 == 0 {
		return fmt.Errorf("SetupDiSetClassInstallParams failed: %w", err)
	}

	r1, _, err = procSetupDiCallClassInstaller.Call(
		DIF_PROPERTYCHANGE,
		h,
		uintptr(unsafe.Pointer(&d.devData)),
	)
	if r1 == 0 {
		return fmt.Errorf("SetupDiCallClassInstaller failed: %w", err)
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
	buf := make([]byte, 512)
	var regType, required uint32
	r1, _, err := procSetupDiGetDeviceRegistryProp.Call(
		h,
		uintptr(unsafe.Pointer(data)),
		SPDRP_DEVICEDESC,
		uintptr(unsafe.Pointer(&regType)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&required)),
	)
	if r1 == 0 {
		return "", fmt.Errorf("SetupDiGetDeviceRegistryProperty failed: %w", err)
	}
	return windows.UTF16ToString((*[256]uint16)(unsafe.Pointer(&buf[0]))[:]), nil
}