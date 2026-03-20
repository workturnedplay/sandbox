package main

import (
	"fmt"
	"log"
//	"os"
	"unsafe"
	"strings"
	"golang.org/x/sys/windows"
)

var (
	setupapi = windows.NewLazySystemDLL("setupapi.dll")
	
	procSetupDiGetClassDevs = setupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInfo = setupapi.NewProc("SetupDiEnumDeviceInfo")
	procSetupDiGetDeviceInstanceId = setupapi.NewProc("SetupDiGetDeviceInstanceIdW")
	procSetupDiGetDeviceRegistryProperty = setupapi.NewProc("SetupDiGetDeviceRegistryPropertyW")
	procSetupDiDestroyDeviceInfoList = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
	procSetupDiSetClassInstallParams = setupapi.NewProc("SetupDiSetClassInstallParamsW")
	procSetupDiCallClassInstaller = setupapi.NewProc("SetupDiCallClassInstaller")
	procSetupDiGetDeviceInstallParams = setupapi.NewProc("SetupDiGetDeviceInstallParamsW")
)

const (
	DIGCF_ALLCLASSES = 0x00000004
	DIGCF_PRESENT = 0x00000002
	SPDRP_DEVICEDESC = 0x00000000
	
	// DIF codes
	DIF_PROPERTYCHANGE = 0x00000012
	
	// State change codes
	DICS_ENABLE = 0x00000001
	DICS_DISABLE = 0x00000002
	DICS_PROPCHANGE = 0x00000003
	
	// Scope flags
	DICS_FLAG_GLOBAL = 0x00000001
	DICS_FLAG_CONFIGSPECIFIC = 0x00000002
	
	// Device install params flags
	DI_NEEDRESTART = 0x00000001
	DI_NEEDREBOOT = 0x00000010
)

type SP_DEVINFO_DATA struct {
	CbSize uint32
	ClassGuid windows.GUID
	DevInst uint32
	Reserved uintptr
}

type SP_CLASSINSTALL_HEADER struct {
	CbSize uint32
	InstallFunction uint32
}

type SP_PROPCHANGE_PARAMS struct {
	ClassInstallHeader SP_CLASSINSTALL_HEADER
	StateChange uint32
	Scope uint32
	HwProfile uint32
}

type SP_DEVINSTALL_PARAMS struct {
	CbSize uint32
	Flags uint32
	FlagsEx uint32
	hwndParent uintptr
	InstallMsgHandler uintptr
	InstallMsgContext uintptr
	FileQueue uintptr
	ClassInstallReserved uintptr
	Reserved uint32
	DriverPath [260]uint16
}

func main() {
	devices, err := listDevices()
	if err != nil {
		log.Fatal(err)
	}
	for i, d := range devices {
		fmt.Printf("[%d] %s\n %s\n", i, d.InstanceID, d.Name)
	}
	var choice int
	fmt.Print("\nSelect device index to restart: ")
	fmt.Scanln(&choice)
	if choice < 0 || choice >= len(devices) {
		log.Fatal("invalid selection")
	}
	err = restartDevice(devices[choice].InstanceID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Device reset issued successfully.")
}

type Device struct {
	InstanceID string
	Name string
}

func listDevices() ([]Device, error) {
	h, _, err := procSetupDiGetClassDevs.Call(
		0,
		0,
		0,
		DIGCF_ALLCLASSES,
	)
	if h == 0 {
		return nil, err
	}
	defer procSetupDiDestroyDeviceInfoList.Call(h)
	
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
		instance, _ := getInstanceID(h, &data)
		name, _ := getDeviceDesc(h, &data)
		if strings.Contains(instance, "VID_1532&PID_0109") {
			devices = append(devices, Device{
				InstanceID: instance,
				Name: name,
			})
		}
	}
	return devices, nil
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
		return "", err
	}
	return windows.UTF16ToString(buf), nil
}

func getDeviceDesc(h uintptr, data *SP_DEVINFO_DATA) (string, error) {
	buf := make([]byte, 512)
	var regType uint32
	var required uint32
	r1, _, err := procSetupDiGetDeviceRegistryProperty.Call(
		h,
		uintptr(unsafe.Pointer(data)),
		SPDRP_DEVICEDESC,
		uintptr(unsafe.Pointer(&regType)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&required)),
	)
	if r1 == 0 {
		return "", err
	}
	return windows.UTF16ToString((*[256]uint16)(unsafe.Pointer(&buf[0]))[:]), nil
}

func restartDevice(instanceID string) error {
	// Get all devices
	h, _, err := procSetupDiGetClassDevs.Call(
		0,
		0,
		0,
		DIGCF_ALLCLASSES,
	)
	if h == 0 {
		return fmt.Errorf("SetupDiGetClassDevs failed: %v", err)
	}
	defer procSetupDiDestroyDeviceInfoList.Call(h)
	
	// Find the device matching our instance ID
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
		
		instance, _ := getInstanceID(h, &data)
		if instance != instanceID {
			continue
		}
		
		// Found our device! Now issue the property change
		return propertyChange(h, &data)
	}
	
	return fmt.Errorf("device not found: %s", instanceID)
}

func propertyChange(h uintptr, devInfo *SP_DEVINFO_DATA) error {
	// Set up the property change params with DICS_PROPCHANGE (cycle the device)
	var pcp SP_PROPCHANGE_PARAMS
	pcp.ClassInstallHeader.CbSize = uint32(unsafe.Sizeof(pcp.ClassInstallHeader))
	pcp.ClassInstallHeader.InstallFunction = DIF_PROPERTYCHANGE
	pcp.StateChange = DICS_PROPCHANGE
	pcp.Scope = DICS_FLAG_CONFIGSPECIFIC
	pcp.HwProfile = 0
	
	// Set the class install params
	r1, _, err := procSetupDiSetClassInstallParams.Call(
		h,
		uintptr(unsafe.Pointer(devInfo)),
		uintptr(unsafe.Pointer(&pcp.ClassInstallHeader)),
		uintptr(unsafe.Sizeof(pcp)),
	)
	if r1 == 0 {
		return fmt.Errorf("SetupDiSetClassInstallParams failed: %v", err)
	}
	
	// Call the class installer
	r1, _, err = procSetupDiCallClassInstaller.Call(
		DIF_PROPERTYCHANGE,
		h,
		uintptr(unsafe.Pointer(devInfo)),
	)
	if r1 == 0 {
		return fmt.Errorf("SetupDiCallClassInstaller failed: %v", err)
	}
	
	// Check if reboot is required
	var devParams SP_DEVINSTALL_PARAMS
	devParams.CbSize = uint32(unsafe.Sizeof(devParams))
	r1, _, err = procSetupDiGetDeviceInstallParams.Call(
		h,
		uintptr(unsafe.Pointer(devInfo)),
		uintptr(unsafe.Pointer(&devParams)),
	)
	if r1 != 0 && (devParams.Flags&DI_NEEDRESTART != 0 || devParams.Flags&DI_NEEDREBOOT != 0) {
		fmt.Println("Note: Device reboot may be required.")
	}
	
	return nil
}
