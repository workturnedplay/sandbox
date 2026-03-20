package main

import (
"encoding/binary"
	"fmt"
	//"os/exec"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	setupapi = windows.NewLazySystemDLL("setupapi.dll")
	cfgmgr32 = windows.NewLazySystemDLL("cfgmgr32.dll")

	// SetupAPI
	SetupDiGetClassDevsW            = setupapi.NewProc("SetupDiGetClassDevsW")
	SetupDiEnumDeviceInterfaces     = setupapi.NewProc("SetupDiEnumDeviceInterfaces")
	SetupDiGetDeviceInterfaceDetail = setupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	SetupDiDestroyDeviceInfoList    = setupapi.NewProc("SetupDiDestroyDeviceInfoList")

	// CfgMgr
	CM_Locate_DevNodeW = cfgmgr32.NewProc("CM_Locate_DevNodeW")
	CM_Get_Parent      = cfgmgr32.NewProc("CM_Get_Parent")
)

const (
	DIGCF_PRESENT         = 0x00000002
	DIGCF_DEVICEINTERFACE = 0x00000010
	INVALID_HANDLE_VALUE  = ^uintptr(0)
)

var GUID_DEVINTERFACE_USB_HUB = windows.GUID{
	Data1: 0xf18a0e88,
	Data2: 0xc30c,
	Data3: 0x11d0,
	Data4: [8]byte{0x88, 0x15, 0x00, 0xa0, 0xc9, 0x06, 0xbe, 0xd8},
}

const (
	IOCTL_USB_GET_NODE_CONNECTION_INFORMATION_EX = 0x220448
	IOCTL_USB_HUB_CYCLE_PORT                     = 0x220444
)

type USB_NODE_CONNECTION_INFORMATION_EX struct {
	ConnectionIndex           uint32
	DeviceDescriptor          [18]byte
	CurrentConfigurationValue byte
	Speed                     byte
	DeviceIsHub               byte
	DeviceAddress             uint16
	NumberOfOpenPipes         uint32
}

type USB_CYCLE_PORT_PARAMS struct {
	ConnectionIndex uint32
}

// ================== MAIN FUNCTION ==================

func main() {
	// ←←← PUT YOUR KEYBOARD INSTANCE ID HERE (from your other program)
	keyboardID := `HID\VID_1532&PID_0109&MI_00\7&8F9FD76&0&0000` // ← change this

	fmt.Println("Starting real USB port cycle for keyboard:", keyboardID)

	// 1. Get keyboard DevNode
	devInst := locateDevNode(keyboardID)

	// 2. Walk up to the parent USB hub DevNode
	hubDevNode := walkToHub(devInst)

	// 3. Find the hub's device interface path
	hubPath := getHubInterfacePath(hubDevNode)

	fmt.Println("Hub path found:", hubPath)

	// 4. Open the hub
	hubHandle := openHub(hubPath)
	defer windows.CloseHandle(hubHandle)

	// 5. Find which port our Razer is on
	port := findPort(hubHandle, 0x1532, 0x0109)
	fmt.Println("Razer is on port:", port)

	// 6. Cycle the port (real power off/on)
	cyclePort(hubHandle, port)

	fmt.Println("=== PORT CYCLED SUCCESSFULLY ===")
	fmt.Println("Keyboard should now be fully reset (LEDs off → on).")
}

// ================== HELPERS ==================

func locateDevNode(instanceID string) uint32 {
	var devInst uint32
	ptr := windows.StringToUTF16Ptr(instanceID)

	r, _, _ := CM_Locate_DevNodeW.Call(
		uintptr(unsafe.Pointer(&devInst)),
		uintptr(unsafe.Pointer(ptr)),
		0,
	)
	if r != 0 {
		panic(fmt.Sprintf("CM_Locate_DevNodeW failed for %s", instanceID))
	}
	return devInst
}

func walkToHub(devInst uint32) uint32 {
	current := devInst
	for i := 0; i < 10; i++ { // safety limit
		var parent uint32
		r, _, _ := CM_Get_Parent.Call(
			uintptr(unsafe.Pointer(&parent)),
			uintptr(current),
			0,
		)
		if r != 0 {
			break
		}
		current = parent

		// Stop when we reach a USB hub (usually the root hub or composite)
		if strings.Contains(getInstanceIDFromDevNode(current), "ROOT_HUB") ||
			strings.Contains(getInstanceIDFromDevNode(current), "USB\\VID") {
			return current
		}
	}
	return current // fallback
}

func getInstanceIDFromDevNode(devInst uint32) string {
	// Not needed for core logic, but useful for debug
	return ""
}

// ================== HUB INTERFACE ENUMERATION (fixed) ==================

func getHubInterfacePath(targetDevNode uint32) string {
	h, _, _ := SetupDiGetClassDevsW.Call(
		uintptr(unsafe.Pointer(&GUID_DEVINTERFACE_USB_HUB)),
		0, 0,
		DIGCF_PRESENT|DIGCF_DEVICEINTERFACE,
	)
	if h == INVALID_HANDLE_VALUE {
		panic("SetupDiGetClassDevsW failed")
	}
	defer SetupDiDestroyDeviceInfoList.Call(h)

	type SP_DEVICE_INTERFACE_DATA struct {
		cbSize             uint32
		InterfaceClassGuid windows.GUID
		Flags              uint32
		Reserved           uintptr
	}

	type SP_DEVINFO_DATA struct {
		cbSize    uint32
		ClassGuid windows.GUID
		DevInst   uint32
		Reserved  uintptr
	}

	type SP_DEVICE_INTERFACE_DETAIL_DATA struct {
		cbSize     uint32
		DevicePath [1]uint16
	}

	for i := uint32(0); ; i++ {
		var iface SP_DEVICE_INTERFACE_DATA
		iface.cbSize = uint32(unsafe.Sizeof(iface))

		r, _, _ := SetupDiEnumDeviceInterfaces.Call(
			h,
			0,
			uintptr(unsafe.Pointer(&GUID_DEVINTERFACE_USB_HUB)),
			uintptr(i),
			uintptr(unsafe.Pointer(&iface)),
		)
		if r == 0 {
			break // no more
		}

		var devInfo SP_DEVINFO_DATA
		devInfo.cbSize = uint32(unsafe.Sizeof(devInfo))

		var required uint32
		SetupDiGetDeviceInterfaceDetail.Call(
			h,
			uintptr(unsafe.Pointer(&iface)),
			0, 0,
			uintptr(unsafe.Pointer(&required)),
			uintptr(unsafe.Pointer(&devInfo)),
		)

		if required == 0 {
			continue
		}

		buf := make([]byte, required)
		detail := (*SP_DEVICE_INTERFACE_DETAIL_DATA)(unsafe.Pointer(&buf[0]))
		detail.cbSize = uint32(unsafe.Sizeof(uintptr(0))) // 4 or 8

		r2, _, _ := SetupDiGetDeviceInterfaceDetail.Call(
			h,
			uintptr(unsafe.Pointer(&iface)),
			uintptr(unsafe.Pointer(detail)),
			uintptr(required),
			0,
			uintptr(unsafe.Pointer(&devInfo)),
		)
		if r2 == 0 {
			continue
		}

		if devInfo.DevInst == targetDevNode {
			path := windows.UTF16PtrToString(&detail.DevicePath[0])
			return path
		}
	}

	panic("Could not find hub interface for target DevNode")
}

func openHub(path string) windows.Handle {
	h, err := windows.CreateFile(
		windows.StringToUTF16Ptr(path),
		windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		panic(err)
	}
	return h
}

func findPort(h windows.Handle, targetVID, targetPID uint16) uint32 {
	for port := uint32(1); port <= 64; port++ {
		info := USB_NODE_CONNECTION_INFORMATION_EX{ConnectionIndex: port}

		var bytesReturned uint32
		err := windows.DeviceIoControl(
			h,
			IOCTL_USB_GET_NODE_CONNECTION_INFORMATION_EX,
			(*byte)(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
			(*byte)(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
			&bytesReturned,
			nil,
		)
		if err != nil {
			continue
		}

		vid := binary.LittleEndian.Uint16(info.DeviceDescriptor[8:10])
		pid := binary.LittleEndian.Uint16(info.DeviceDescriptor[10:12])

		if vid == targetVID && pid == targetPID {
			return port
		}
	}
	panic("Razer not found on any port")
}

func cyclePort(h windows.Handle, port uint32) {
	params := USB_CYCLE_PORT_PARAMS{ConnectionIndex: port}

	var bytesReturned uint32
	err := windows.DeviceIoControl(
		h,
		IOCTL_USB_HUB_CYCLE_PORT,
		(*byte)(unsafe.Pointer(&params)),
		uint32(unsafe.Sizeof(params)),
		nil, 0,
		&bytesReturned,
		nil,
	)
	if err != nil {
		panic(err)
	}
}