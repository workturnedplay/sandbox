/*
The "Hang-o-Matic" Test Program (hang_test.go)

This program will run for 5 seconds, then "Hang" for 3 seconds. During the hang, it will not respond to any Windows messages, allowing you to test if your AttachThreadInput logic successfully detects the unresponsiveness.

I used this to test winbollocks on
*/
package main

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Constants
const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WM_DESTROY          = 0x0002
	WM_APP_HANG         = 0x8001
)

// Structs
type WndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type Msg struct {
	Hwnd    windows.Handle
	Message uint32
	Wparam  uintptr
	Lparam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

var (
	user32 = windows.NewLazySystemDLL("user32.dll")

	procRegisterClassExW  = user32.NewProc("RegisterClassExW")
	procCreateWindowExW   = user32.NewProc("CreateWindowExW")
	procDefWindowProcW    = user32.NewProc("DefWindowProcW")
	procPostQuitMessage   = user32.NewProc("PostQuitMessage")
	procGetMessageW       = user32.NewProc("GetMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessageW  = user32.NewProc("DispatchMessageW")
	procSendMessageW      = user32.NewProc("SendMessageW")
	procSetWindowTextW    = user32.NewProc("SetWindowTextW")
	procUpdateWindow      = user32.NewProc("UpdateWindow")
)

func main() {
	runtime.LockOSThread()

	className, _ := windows.UTF16PtrFromString("HangTestClass")
	windowName, _ := windows.UTF16PtrFromString("STATUS: RUNNING")

	wc := WndClassExW{
		WndProc:   windows.NewCallback(wndProc),
		ClassName: className,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		WS_OVERLAPPEDWINDOW|WS_VISIBLE,
		100, 100, 400, 200,
		0, 0, 0, 0,
	)

	// Background ticker to trigger the hang
	go func() {
		for {
			time.Sleep(5 * time.Second)
			
			// Update title before hanging
			titleHang, _ := windows.UTF16PtrFromString("STATUS: !! HUNG !!")
			procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(titleHang)))
			procUpdateWindow.Call(hwnd)
			
			fmt.Printf("Target is hanging for %d seconds...\n", HANGSECONDS)
			procSendMessageW.Call(hwnd, WM_APP_HANG, 0, 0)
		}
	}()

	var msg Msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

const HANGSECONDS = 5
func wndProc(hwnd windows.Handle, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case WM_APP_HANG:
		time.Sleep(HANGSECONDS * time.Second)
		titleRun, _ := windows.UTF16PtrFromString("STATUS: RUNNING")
		procSetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(titleRun)))
		return 0
	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	default:
		ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wparam, lparam)
		return ret
	}
}