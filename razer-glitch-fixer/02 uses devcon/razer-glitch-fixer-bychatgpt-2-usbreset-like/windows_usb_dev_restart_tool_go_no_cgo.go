package main

import (
	"fmt"
	"log"
	"os"
    "os/exec"
	"unsafe"
  "strings"

	"golang.org/x/sys/windows"
)

var (
	setupapi                     = windows.NewLazySystemDLL("setupapi.dll")
	procSetupDiGetClassDevs     = setupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInfo   = setupapi.NewProc("SetupDiEnumDeviceInfo")
	procSetupDiGetDeviceInstanceId = setupapi.NewProc("SetupDiGetDeviceInstanceIdW")
	procSetupDiGetDeviceRegistryProperty = setupapi.NewProc("SetupDiGetDeviceRegistryPropertyW")
	procSetupDiDestroyDeviceInfoList = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
)

const (
	DIGCF_ALLCLASSES = 0x00000004
	DIGCF_PRESENT    = 0x00000002

	SPDRP_DEVICEDESC = 0x00000000
)

type SP_DEVINFO_DATA struct {
	CbSize    uint32
	ClassGuid windows.GUID
	DevInst   uint32
	Reserved  uintptr
}

func main() {
	devices, err := listDevices()
	if err != nil {
		log.Fatal(err)
	}

	for i, d := range devices {
		fmt.Printf("[%d] %s\n    %s\n", i, d.InstanceID, d.Name)
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

	fmt.Println("Restart issued.")
}

type Device struct {
	InstanceID string
	Name       string
}

func listDevices() ([]Device, error) {
	h, _, err := procSetupDiGetClassDevs.Call(
		0,
		0,
		0,
		DIGCF_ALLCLASSES, //|DIGCF_PRESENT,
    
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
			Name:       name,
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

// NOTE: simplified: just shells out to devcon for now
func restartDevice(instance string) error {
	//cmd := fmt.Sprintf(".\\devcon64.exe restart @\"%s\"", instance)
	//return syscall.Exec("cmd.exe", []string{"cmd.exe", "/C", cmd}, nil)
  //fmt.Println("Using: ",instance)
  cmd := exec.Command(".\\devcon64.exe", "restart", "@"+instance,)
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
fmt.Printf("ARGS: %#v\n", cmd.Args)
return cmd.Run()
}
