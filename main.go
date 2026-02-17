package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	procSetWindowsHookEx  = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx    = user32.NewProc("CallNextHookEx")     // ← use this if defined
	procUnhookWindowsHook = user32.NewProc("UnhookWindowsHookEx")
	procGetMessage        = user32.NewProc("GetMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessage   = user32.NewProc("DispatchMessageW")
	procEnumWindows       = user32.NewProc("EnumWindows")
	procGetWindowText     = user32.NewProc("GetWindowTextW")
	procGetWindowThread   = user32.NewProc("GetWindowThreadProcessId")

	hook windows.Handle
)

// Manual POINT definition (since package export is unreliable in your version)
type POINT struct {
	X, Y int32
}

type MSG struct {
	Hwnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT   // ← your manual type
}

type CWPSTRUCT struct {
	LParam  uintptr
	WParam  uintptr
	Message uint32
	Hwnd    windows.Handle
}

func hookCallback(code int, wParam uintptr, lParam uintptr) uintptr {
	if code >= 0 {
		cwp := (*CWPSTRUCT)(unsafe.Pointer(lParam))
		fmt.Printf("Msg: 0x%X, Hwnd: 0x%X, wParam: 0x%X, lParam: 0x%X\n",
			cwp.Message, cwp.Hwnd, cwp.WParam, cwp.LParam)
	}

	// Use pre-defined procCallNextHookEx (cheaper)
	r1, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
	return r1
}

func enumCallback(hwnd uintptr, lParam uintptr) uintptr {
	var buf [256]uint16
	procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	title := windows.UTF16ToString(buf[:])
	if title != "" {
		fmt.Printf("HWND: 0x%X, Title: %s\n", hwnd, title)
	}
	return 1 // continue
}

func main() {
	runtime.LockOSThread() // Required for hooks

	// Show list of windows
	fmt.Println("Enumerating top-level windows...")
	procEnumWindows.Call(syscall.NewCallback(enumCallback), 0)

	// Get target HWND from user
	var hwndStr string
	fmt.Print("\nEnter target HWND (hex, e.g., 0x00123456): ")
	fmt.Scan(&hwndStr)

	hwnd64, err := strconv.ParseUint(hwndStr, 0, 64)
	if err != nil {
		fmt.Printf("Invalid HWND: %v\n", err)
		return
	}
	hwnd := uintptr(hwnd64) // explicit cast to uintptr

	// Get thread ID of target window
	var tid uint32
	procGetWindowThread.Call(hwnd, uintptr(unsafe.Pointer(&tid)))

	// Install hook on that thread
	cb := syscall.NewCallback(hookCallback)
	h, _, err := procSetWindowsHookEx.Call(
		4, // WH_CALLWNDPROC
		cb,
		0,
		uintptr(tid),
	)
	if h == 0 {
		fmt.Printf("SetWindowsHookEx failed: %v\n", err)
		return
	}
	hook = windows.Handle(h)
	defer procUnhookWindowsHook.Call(uintptr(hook))

	// Graceful shutdown on Ctrl+C
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\nShutting down hook...")
		os.Exit(0)
	}()

	fmt.Printf("Spying on HWND 0x%X (thread %d)... Press Ctrl+C to stop.\n", hwnd, tid)

	// Message pump to keep program alive
	var msg MSG
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}