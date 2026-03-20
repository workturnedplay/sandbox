package selfcheck

import (
	"syscall"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW  = user32.NewProc("MessageBoxW")
)

const (
	mbOKCancel     = 0x00000001
	mbIconWarning = 0x00000030
	mbIconError   = 0x00000010
	mbDefButton2  = 0x00000100

	idOK     = 1
	idCancel = 2
)

func showDialog(title, text string, allowContinue bool) {
	flags := mbIconWarning | mbDefButton2
	if allowContinue {
		flags |= mbOKCancel
	} else {
		flags = mbIconError
	}

	ret, _, _ := procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(text))),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(title))),
		uintptr(flags),
	)

	if allowContinue && ret == idCancel {
		syscall.ExitProcess(1)
	}
}
