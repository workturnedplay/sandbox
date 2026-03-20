package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	cfgmgr32 = windows.NewLazySystemDLL("cfgmgr32.dll")
	CM_Locate_DevNodeW = cfgmgr32.NewProc("CM_Locate_DevNodeW")
	CM_Get_Parent      = cfgmgr32.NewProc("CM_Get_Parent")
)

func main() {
	keyboardID := `HID\VID_1532&PID_0109&MI_00\7&8F9FD76&0&0000`

	fmt.Println("Keyboard ID:", keyboardID)

	devInst := locateDevNode(keyboardID)
	fmt.Printf("Keyboard DevNode: %d (0x%X)\n", devInst, devInst)

	current := devInst
	for i := 0; i < 20; i++ {
		var parent uint32
		r, _, _ := CM_Get_Parent.Call(
			uintptr(unsafe.Pointer(&parent)),
			uintptr(current),
			0,
		)
		if r != 0 {
			fmt.Println("No more parent")
			break
		}
		current = parent
		fmt.Printf("Parent %d: DevNode %d (0x%X)\n", i+1, current, current)
	}
}

func locateDevNode(instanceID string) uint32 {
	var devInst uint32
	ptr := windows.StringToUTF16Ptr(instanceID)

	r, _, _ := CM_Locate_DevNodeW.Call(
		uintptr(unsafe.Pointer(&devInst)),
		uintptr(unsafe.Pointer(ptr)),
		0,
	)
	if r != 0 {
		panic("CM_Locate_DevNodeW failed")
	}
	return devInst
}