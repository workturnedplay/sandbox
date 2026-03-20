package main

import (
	"fmt"
	"unsafe"
  "strings"

	"golang.org/x/sys/windows"
)

const (
	IOCTL_USB_HUB_CYCLE_PORT = 0x220444
)

type USB_CYCLE_PORT_PARAMS struct {
	ConnectionIndex uint32
}

func main() {
	// Hardcode the composite USB device
	compositeID := `USB\VID_1532&PID_0109\5&1E7D8DB7&0&14`

	fmt.Println("Trying direct cycle on composite device:", compositeID)

	// Convert InstanceId to device path (this is a guess - may not work)
	devicePath := `\\?\` + strings.ReplaceAll(compositeID, `\`, `#`) + `#{f18a0e88-c30c-11d0-8815-00a0c906bed8}`

	fmt.Println("Trying device path:", devicePath)

	h, err := windows.CreateFile(
		windows.StringToUTF16Ptr(devicePath),
		windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		fmt.Println("Failed to open device:", err)
		return
	}
	defer windows.CloseHandle(h)

	fmt.Println("Device opened successfully")

	// Try to cycle (this may fail if it's not a hub)
	params := USB_CYCLE_PORT_PARAMS{ConnectionIndex: 0} // port 0 is often the device itself

	var bytesReturned uint32
	err = windows.DeviceIoControl(
		h,
		IOCTL_USB_HUB_CYCLE_PORT,
		(*byte)(unsafe.Pointer(&params)),
		uint32(unsafe.Sizeof(params)),
		nil, 0,
		&bytesReturned,
		nil,
	)
	if err != nil {
		fmt.Println("Cycle failed:", err)
	} else {
		fmt.Println("Cycle command sent successfully!")
	}
}