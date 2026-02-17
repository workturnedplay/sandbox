package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	procSetWindowsHookEx  = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx    = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHook = user32.NewProc("UnhookWindowsHookEx")
	procGetMessage        = user32.NewProc("GetMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessage   = user32.NewProc("DispatchMessageW")
	procEnumWindows       = user32.NewProc("EnumWindows")
	procGetWindowText     = user32.NewProc("GetWindowTextW")
	procGetWindowThread   = user32.NewProc("GetWindowThreadProcessId")

	hook windows.Handle
)

type MSG struct {
	Hwnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      windows.POINT
}

func hookCallback(code int, wParam uintptr, lParam uintptr) uintptr {
	if code >= 0 {
		cwp := (*CWPSTRUCT)(unsafe.Pointer(lParam))
		fmt.Printf("Msg: 0x%X, Hwnd: 0x%X, wParam: 0x%X, lParam: 0x%X\n", cwp.Message, cwp.Hwnd, cwp.WParam, cwp.LParam)
	}
	return syscall.MustLoadDLL("user32.dll").MustFindProc("CallNextHookEx").Call(0, uintptr(code), wParam, lParam)
}

type CWPSTRUCT struct {
	LParam uintptr
	WParam uintptr
	Message uint32
	Hwnd windows.Handle
}

func enumCallback(hwnd uintptr, lParam uintptr) uintptr {
	var len uint32 = 256
	buf := make([]uint16, len)
	procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len))
	title := windows.UTF16ToString(buf)
	if title != "" {
		fmt.Printf("HWND: 0x%X, Title: %s\n", hwnd, title)
	}
	return 1 // Continue enum
}

func main() {
	runtime.LockOSThread() // Pin main thread

	// Enum windows to pick target
	procEnumWindows.Call(syscall.NewCallback(enumCallback), 0)

	var hwndStr string
	fmt.Print("Enter target HWND (hex, e.g., 0x123456): ")
	fmt.Scan(&hwndStr)
	hwnd, _ := strconv.ParseUint(hwndStr, 0, 64) // Hex parse

	var tid uint32
	procGetWindowThread.Call(hwnd, uintptr(unsafe.Pointer(&tid)))

	// Install thread hook
	cb := syscall.NewCallback(hookCallback)
	h, _, err := procSetWindowsHookEx.Call(4 /* WH_CALLWNDPROC */, cb, 0, uintptr(tid))
	if h == 0 {
		fmt.Printf("Hook failed: %v\n", err)
		return
	}
	hook = windows.Handle(h)
	defer procUnhookWindowsHook.Call(uintptr(hook))

	// Ctrl+C cleanup
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		os.Exit(0)
	}()

	// Message loop
	var msg MSG
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) == 0 { break }
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}