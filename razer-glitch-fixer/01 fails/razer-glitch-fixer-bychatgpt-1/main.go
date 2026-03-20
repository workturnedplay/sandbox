package main

import (
	"encoding/binary"
	"fmt"
	"golang.org/x/sys/windows"
	"strings"
	"unsafe"
)

const (
	DIGCF_PRESENT         = 0x2
	DIGCF_DEVICEINTERFACE = 0x10

	IOCTL_USB_GET_NODE_CONNECTION_INFORMATION_EX = 0x220448
	IOCTL_USB_HUB_CYCLE_PORT                     = 0x220444
)

type USB_CYCLE_PORT_PARAMS struct {
	ConnectionIndex uint32
}

type USB_NODE_CONNECTION_INFORMATION_EX struct {
	ConnectionIndex           uint32
	DeviceDescriptor          [18]byte
	CurrentConfigurationValue byte
	Speed                     byte
	DeviceIsHub               byte
	DeviceAddress             uint16
	NumberOfOpenPipes         uint32
}

var (
	cfgmgr   = windows.NewLazySystemDLL("cfgmgr32.dll")
	setupapi = windows.NewLazySystemDLL("setupapi.dll")

	CM_Locate_DevNodeW = cfgmgr.NewProc("CM_Locate_DevNodeW")
	CM_Get_Parent      = cfgmgr.NewProc("CM_Get_Parent")

	SetupDiGetClassDevsW            = setupapi.NewProc("SetupDiGetClassDevsW")
	SetupDiEnumDeviceInterfaces     = setupapi.NewProc("SetupDiEnumDeviceInterfaces")
	SetupDiGetDeviceInterfaceDetail = setupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	SetupDiGetDeviceInstanceIdW     = setupapi.NewProc("SetupDiGetDeviceInstanceIdW")

	SetupDiDestroyDeviceInfoList = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
)

var GUID_DEVINTERFACE_USB_HUB = windows.GUID{
	Data1: 0xf18a0e88,
	Data2: 0xc30c,
	Data3: 0x11d0,
	Data4: [8]byte{0x88, 0x15, 0x00, 0xa0, 0xc9, 0x06, 0xbe, 0xd8},
}

func locateDevNode(instance string) uint32 {

	var devInst uint32

	ptr := windows.StringToUTF16Ptr(instance)

	r, _, _ := CM_Locate_DevNodeW.Call(
		uintptr(unsafe.Pointer(&devInst)),
		uintptr(unsafe.Pointer(ptr)),
		0,
	)

	if r != 0 {
		panic("CM_Locate_DevNode failed")
	}

	return devInst
}

func getParent(devInst uint32) uint32 {

	var parent uint32

	r, _, _ := CM_Get_Parent.Call(
		uintptr(unsafe.Pointer(&parent)),
		uintptr(devInst),
		0,
	)

	if r != 0 {
		panic("CM_Get_Parent failed")
	}

	return parent
}

// func getHubInterfacePath() string {

// h, _, _ := SetupDiGetClassDevsW.Call(
// uintptr(unsafe.Pointer(&GUID_DEVINTERFACE_USB_HUB)),
// 0,
// 0,
// DIGCF_PRESENT|DIGCF_DEVICEINTERFACE,
// )

// if h == uintptr(windows.InvalidHandle) {
// panic("SetupDiGetClassDevs failed")
// }

// defer SetupDiDestroyDeviceInfoList.Call(h)

// type SP_DEVICE_INTERFACE_DATA struct {
// cbSize             uint32
// InterfaceClassGuid windows.GUID
// Flags              uint32
// Reserved           uintptr
// }

// var iface SP_DEVICE_INTERFACE_DATA
// iface.cbSize = uint32(unsafe.Sizeof(iface))

// r, _, _ := SetupDiEnumDeviceInterfaces.Call(
// h,
// 0,
// uintptr(unsafe.Pointer(&GUID_DEVINTERFACE_USB_HUB)),
// 0,
// uintptr(unsafe.Pointer(&iface)),
// )

// if r == 0 {
// panic("SetupDiEnumDeviceInterfaces failed")
// }

// var required uint32

// SetupDiGetDeviceInterfaceDetail.Call(
// h,
// uintptr(unsafe.Pointer(&iface)),
// 0,
// 0,
// uintptr(unsafe.Pointer(&required)),
// 0,
// )

// buf := make([]byte, required)

// type SP_DEVICE_INTERFACE_DETAIL_DATA struct {
// cbSize     uint32
// DevicePath uint16
// }

// detail := (*SP_DEVICE_INTERFACE_DETAIL_DATA)(unsafe.Pointer(&buf[0]))
// detail.cbSize = 8

// r, _, _ = SetupDiGetDeviceInterfaceDetail.Call(
// h,
// uintptr(unsafe.Pointer(&iface)),
// uintptr(unsafe.Pointer(detail)),
// uintptr(required),
// 0,
// 0,
// )

// if r == 0 {
// panic("SetupDiGetDeviceInterfaceDetail failed")
// }

// path := windows.UTF16PtrToString(&detail.DevicePath)

// return path
// }

// func getHubInterfacePath(targetHubDevNode uint32) string {
// h, _, _ := SetupDiGetClassDevsW.Call(
// uintptr(unsafe.Pointer(&GUID_DEVINTERFACE_USB_HUB)),
// 0,
// 0,
// DIGCF_PRESENT|DIGCF_DEVICEINTERFACE,
// )
// if h == uintptr(windows.InvalidHandle) {
// panic("SetupDiGetClassDevs failed")
// }
// defer SetupDiDestroyDeviceInfoList.Call(h)

// type SP_DEVICE_INTERFACE_DATA struct {
// cbSize             uint32
// InterfaceClassGuid windows.GUID
// Flags              uint32
// Reserved           uintptr
// }

// type SP_DEVINFO_DATA struct {
// cbSize    uint32
// ClassGuid windows.GUID
// DevInst   uint32
// Reserved  uintptr
// }

// type SP_DEVICE_INTERFACE_DETAIL_DATA struct {
// cbSize     uint32
// DevicePath [1]uint16
// }

// const maxIndex = 128
// for i := 0; i < maxIndex; i++ {
// var iface SP_DEVICE_INTERFACE_DATA
// iface.cbSize = uint32(unsafe.Sizeof(iface))

// r, _, _ := SetupDiEnumDeviceInterfaces.Call(
// h,
// 0,
// uintptr(unsafe.Pointer(&GUID_DEVINTERFACE_USB_HUB)),
// uintptr(i),
// uintptr(unsafe.Pointer(&iface)),
// )
// if r == 0 {
// break // no more interfaces
// }

// var devInfo SP_DEVINFO_DATA
// devInfo.cbSize = uint32(unsafe.Sizeof(devInfo))

// var required uint32
// SetupDiGetDeviceInterfaceDetail.Call(
// h,
// uintptr(unsafe.Pointer(&iface)),
// 0,                // detail buffer is nil for first call
// 0,                // detail buffer size
// uintptr(unsafe.Pointer(&required)),
// uintptr(unsafe.Pointer(&devInfo)), // <- pass real devInfo here
// )
// //Allocate buffer and make second call to actually get the device path:
// buf := make([]byte, required)
// detail := (*SP_DEVICE_INTERFACE_DETAIL_DATA)(unsafe.Pointer(&buf[0]))
// if unsafe.Sizeof(uintptr(0)) == 8 {
// detail.cbSize = 8
// } else {
// detail.cbSize = 6
// }
// // Query DevNode of this interface
// r2, _, _ := SetupDiGetDeviceInterfaceDetail.Call(
// h,
// uintptr(unsafe.Pointer(&iface)),
// uintptr(unsafe.Pointer(detail)),
// uintptr(required),
// 0,
// uintptr(unsafe.Pointer(&devInfo)),
// )
// if r2 == 0 {
// continue
// }

// if devInfo.DevInst == targetHubDevNode {
// // Get required buffer size
// var required uint32
// SetupDiGetDeviceInterfaceDetail.Call(
// h,
// uintptr(unsafe.Pointer(&iface)),
// 0,
// 0,
// uintptr(unsafe.Pointer(&required)),
// 0,
// )

// buf := make([]byte, required)
// detail := (*SP_DEVICE_INTERFACE_DETAIL_DATA)(unsafe.Pointer(&buf[0]))
// if unsafe.Sizeof(uintptr(0)) == 8 { // 64-bit
// detail.cbSize = 8
// } else {
// detail.cbSize = 6 // 32-bit
// }

// r3, _, _ := SetupDiGetDeviceInterfaceDetail.Call(
// h,
// uintptr(unsafe.Pointer(&iface)),
// uintptr(unsafe.Pointer(detail)),
// uintptr(required),
// 0,
// 0,
// )
// if r3 == 0 {
// panic("SetupDiGetDeviceInterfaceDetail failed")
// }

// path := windows.UTF16PtrToString(&detail.DevicePath[0])
// return path
// }
// }

// panic("hub interface for target DevNode not found")
// }

// getHubInterfacePath finds the hub device interface path whose device InstanceId
// equals targetHubInstance (example: "USB\\ROOT_HUB30\\4&186df573&0&0").
// It returns the device path (\\?\usb#root_hub30#...) or panics on error.
// func getHubInterfacePath(targetHubInstance string) string {
// 	h, _, _ := SetupDiGetClassDevsW.Call(
// 		uintptr(unsafe.Pointer(&GUID_DEVINTERFACE_USB_HUB)),
// 		0,
// 		0,
// 		DIGCF_PRESENT|DIGCF_DEVICEINTERFACE,
// 	)
// 	if h == uintptr(windows.InvalidHandle) {
// 		panic("SetupDiGetClassDevsW failed")
// 	}
// 	defer SetupDiDestroyDeviceInfoList.Call(h)

// 	type SP_DEVICE_INTERFACE_DATA struct {
// 		cbSize             uint32
// 		InterfaceClassGuid windows.GUID
// 		Flags              uint32
// 		Reserved           uintptr
// 	}
// 	type SP_DEVINFO_DATA struct {
// 		cbSize    uint32
// 		ClassGuid windows.GUID
// 		DevInst   uint32
// 		Reserved  uintptr
// 	}
// 	type SP_DEVICE_INTERFACE_DETAIL_DATA struct {
// 		cbSize     uint32
// 		DevicePath [1]uint16
// 	}

// 	// enumerate interfaces
// 	for idx := 0; ; idx++ {
// 		var iface SP_DEVICE_INTERFACE_DATA
// 		iface.cbSize = uint32(unsafe.Sizeof(iface))

// 		r, _, _ := SetupDiEnumDeviceInterfaces.Call(
// 			h,
// 			0,
// 			uintptr(unsafe.Pointer(&GUID_DEVINTERFACE_USB_HUB)),
// 			uintptr(idx),
// 			uintptr(unsafe.Pointer(&iface)),
// 		)
// 		if r == 0 {
// 			// no more interfaces
// 			break
// 		}

// 		// Prepare devInfo for receiving the instance id and for the detail call
// 		var devInfo SP_DEVINFO_DATA
// 		devInfo.cbSize = uint32(unsafe.Sizeof(devInfo))

// 		// First call to get required buffer size and also populate devInfo via last param.
// 		var required uint32
// 		r1, _, _ := SetupDiGetDeviceInterfaceDetail.Call(
// 			h,
// 			uintptr(unsafe.Pointer(&iface)),
// 			0,
// 			0,
// 			uintptr(unsafe.Pointer(&required)),
// 			uintptr(unsafe.Pointer(&devInfo)), // ask SetupDi to fill devInfo
// 		)
// 		if r1 == 0 && windows.GetLastError() == windows.ERROR_INSUFFICIENT_BUFFER {
// 			// expected: required contains needed size
// 		} else if r1 == 0 {
// 			// other failure; skip this interface
// 			continue
// 		}

// 		// Allocate buffer and get full device interface detail (including device path)
// 		buf := make([]byte, required)
// 		detail := (*SP_DEVICE_INTERFACE_DETAIL_DATA)(unsafe.Pointer(&buf[0]))
// 		if unsafe.Sizeof(uintptr(0)) == 8 { // 64-bit
// 			detail.cbSize = 8
// 		} else {
// 			detail.cbSize = 6 // 32-bit
// 		}

// 		r2, _, _ := SetupDiGetDeviceInterfaceDetail.Call(
// 			h,
// 			uintptr(unsafe.Pointer(&iface)),
// 			uintptr(unsafe.Pointer(detail)),
// 			uintptr(required),
// 			0,
// 			uintptr(unsafe.Pointer(&devInfo)), // devInfo filled again
// 		)
// 		if r2 == 0 {
// 			// failed to get detail; skip
// 			continue
// 		}

// 		// Now get the device instance id string for this interface's devInfo
// 		// Call SetupDiGetDeviceInstanceIdW twice: first for required size
// 		var reqSize uint32
// 		r3, _, _ := SetupDiGetDeviceInstanceIdW.Call(
// 			h,
// 			uintptr(unsafe.Pointer(&devInfo)),
// 			0,
// 			0,
// 			uintptr(unsafe.Pointer(&reqSize)),
// 		)
// 		if r3 == 0 && windows.GetLastError() != windows.ERROR_INSUFFICIENT_BUFFER && reqSize == 0 {
// 			// failed; skip
// 			continue
// 		}

// 		// allocate buffer for instance id (UTF-16)
// 		idBuf := make([]uint16, reqSize+1)
// 		r4, _, _ := SetupDiGetDeviceInstanceIdW.Call(
// 			h,
// 			uintptr(unsafe.Pointer(&devInfo)),
// 			uintptr(unsafe.Pointer(&idBuf[0])),
// 			uintptr(len(idBuf)),
// 			0,
// 		)
// 		if r4 == 0 {
// 			// failed; skip
// 			continue
// 		}
// 		inst := windows.UTF16ToString(idBuf)

// 		// Compare instance strings case-insensitively.
// 		if strings.EqualFold(inst, targetHubInstance) {
// 			// return the device path from detail.DevicePath
// 			return windows.UTF16PtrToString(&detail.DevicePath[0])
// 		}
// 	}

// 	panic("hub interface for target InstanceId not found")
// }

func getHubInterfacePath(targetHubInstance string) string {
	h, _, _ := SetupDiGetClassDevsW.Call(
		uintptr(unsafe.Pointer(&GUID_DEVINTERFACE_USB_HUB)),
		0,
		0,
		DIGCF_PRESENT|DIGCF_DEVICEINTERFACE,
	)
	if h == uintptr(windows.InvalidHandle) {
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

	fmt.Println("Enumerating hub interfaces...")

	for idx := 0; ; idx++ {
		fmt.Printf("=== Interface index %d ===\n", idx)
		var iface SP_DEVICE_INTERFACE_DATA
		iface.cbSize = uint32(unsafe.Sizeof(iface))

		r, _, _ := SetupDiEnumDeviceInterfaces.Call(
			h,
			0,
			uintptr(unsafe.Pointer(&GUID_DEVINTERFACE_USB_HUB)),
			uintptr(idx),
			uintptr(unsafe.Pointer(&iface)),
		)
		if r == 0 {
			fmt.Println("No more interfaces at this index; breaking")
			break
		}

		var devInfo SP_DEVINFO_DATA
		devInfo.cbSize = uint32(unsafe.Sizeof(devInfo))

		var required uint32
		r1, _, _ := SetupDiGetDeviceInterfaceDetail.Call(
			h,
			uintptr(unsafe.Pointer(&iface)),
			0,
			0,
			uintptr(unsafe.Pointer(&required)),
			uintptr(unsafe.Pointer(&devInfo)),
		)
		fmt.Printf("First SetupDiGetDeviceInterfaceDetail: r1=%d, required=%d, DevInst=%d\n", r1, required, devInfo.DevInst)
		if r1 == 0 && windows.GetLastError() != windows.ERROR_INSUFFICIENT_BUFFER {
			fmt.Println("Skipping interface due to unexpected error on first call")
			continue
		}

		buf := make([]byte, required)
		detail := (*SP_DEVICE_INTERFACE_DETAIL_DATA)(unsafe.Pointer(&buf[0]))
		if unsafe.Sizeof(uintptr(0)) == 8 {
			detail.cbSize = 8
		} else {
			detail.cbSize = 6
		}

		r2, _, _ := SetupDiGetDeviceInterfaceDetail.Call(
			h,
			uintptr(unsafe.Pointer(&iface)),
			uintptr(unsafe.Pointer(detail)),
			uintptr(required),
			0,
			uintptr(unsafe.Pointer(&devInfo)),
		)
		fmt.Printf("Second SetupDiGetDeviceInterfaceDetail: r2=%d, DevInst=%d\n", r2, devInfo.DevInst)
		if r2 == 0 {
			fmt.Println("Skipping interface due to failure on second call")
			continue
		}

		var reqSize uint32
		r3, _, _ := SetupDiGetDeviceInstanceIdW.Call(
			h,
			uintptr(unsafe.Pointer(&devInfo)),
			0,
			0,
			uintptr(unsafe.Pointer(&reqSize)),
		)
		fmt.Printf("SetupDiGetDeviceInstanceIdW first call: r3=%d, reqSize=%d\n", r3, reqSize)
		if r3 == 0 && windows.GetLastError() != windows.ERROR_INSUFFICIENT_BUFFER && reqSize == 0 {
			fmt.Println("Skipping interface: cannot get instance id size")
			continue
		}

		idBuf := make([]uint16, reqSize+1)
		r4, _, _ := SetupDiGetDeviceInstanceIdW.Call(
			h,
			uintptr(unsafe.Pointer(&devInfo)),
			uintptr(unsafe.Pointer(&idBuf[0])),
			uintptr(len(idBuf)),
			0,
		)
		if r4 == 0 {
			fmt.Println("Skipping interface: failed to get instance id")
			continue
		}

		inst := windows.UTF16ToString(idBuf)
		fmt.Printf("Found interface DevNode=%d, InstanceId=%s\n", devInfo.DevInst, inst)

		if strings.EqualFold(inst, targetHubInstance) {
			path := windows.UTF16PtrToString(&detail.DevicePath[0])
			fmt.Printf("Matched target! DevicePath=%s\n", path)
			return path
		} else {
			fmt.Println("InstanceId does not match target, skipping")
		}
	}

	panic("hub interface for target InstanceId not found")
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

func readVidPid(desc [18]byte) (uint16, uint16) {

	vid := binary.LittleEndian.Uint16(desc[8:10])
	pid := binary.LittleEndian.Uint16(desc[10:12])

	return vid, pid
}

func findPort(h windows.Handle, vidTarget, pidTarget uint16) uint32 {

	for port := uint32(1); port <= 32; port++ {

		info := USB_NODE_CONNECTION_INFORMATION_EX{
			ConnectionIndex: port,
		}

		var ret uint32

		err := windows.DeviceIoControl(
			h,
			IOCTL_USB_GET_NODE_CONNECTION_INFORMATION_EX,
			(*byte)(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
			(*byte)(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
			&ret,
			nil,
		)

		if err != nil {
			continue
		}

		vid, pid := readVidPid(info.DeviceDescriptor)

		if vid == vidTarget && pid == pidTarget {
			return port
		}
	}

	panic("device not found on hub")
}

func cyclePort(h windows.Handle, port uint32) {

	params := USB_CYCLE_PORT_PARAMS{
		ConnectionIndex: port,
	}

	var ret uint32

	err := windows.DeviceIoControl(
		h,
		IOCTL_USB_HUB_CYCLE_PORT,
		(*byte)(unsafe.Pointer(&params)),
		uint32(unsafe.Sizeof(params)),
		nil,
		0,
		&ret,
		nil,
	)

	if err != nil {
		panic(err)
	}
}

func main() {

	//instance := `USB\VID_1532&PID_0109&MI_00\6&32DE58AC&0&0000`
	//Hub DevNode: 3
	//Hub path: \\?\usb#root_hub30#5&23b3364d&0&0#{f18a0e88-c30c-11d0-8815-00a0c906bed8}
	//panic: device not found on hub

	//instance := `USB\VID_1532&PID_0109&MI_00\6&32DE58AC&0&0000`
	//Hub DevNode: 3
	//Hub path: \\?\usb#root_hub30#5&23b3364d&0&0#{f18a0e88-c30c-11d0-8815-00a0c906bed8}
	//panic: device not found on hub

	//instance:=`USB\VID_1532&PID_0109&MI_01\6&1DBACD0E&0&0001` // panic: CM_Locate_DevNode failed

	instance := `USB\VID_1532&PID_0109\5&1E7D8DB7&0&14`
	//Hub DevNode: 3
	//Hub path: \\?\usb#root_hub30#5&23b3364d&0&0#{f18a0e88-c30c-11d0-8815-00a0c906bed8}
	//panic: device not found on hub

	dev := locateDevNode(instance)

	parent1 := getParent(dev)
	parent2 := getParent(parent1)

	fmt.Println("Hub DevNode:", parent2)

	target := `USB\ROOT_HUB30\4&186df573&0&0`
	hubPath := getHubInterfacePath(target)

	fmt.Println("Hub path:", hubPath)

	//hubPath = `\\?\usb#root_hub30#4&186df573&0&0#{f18a0e88-c30c-11d0-8815-00a0c906bed8}`
	hub := openHub(hubPath)
	defer windows.CloseHandle(hub)

	port := findPort(hub, 0x1532, 0x0109)

	fmt.Println("Keyboard port:", port)

	cyclePort(hub, port)

	fmt.Println("USB port cycled")
}
