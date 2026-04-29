/*
The "Hang-o-Matic" Test Program (hang_test.go)

This program will run for 5 seconds, then "Hang" for 3 seconds. During the hang, it will not respond to any Windows messages, allowing you to test if your AttachThreadInput logic successfully detects the unresponsiveness.
*/
package main

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	procSetWindowText = user32.NewProc("SetWindowTextW")
	procUpdateWindow  = user32.NewProc("UpdateWindow")
)

func main() {
	// Lock to main thread for GUI
	runtime.LockOSThread()

	className, _ := windows.UTF16PtrFromString("HangTestClass")
	windowName, _ := windows.UTF16PtrFromString("STATUS: RUNNING")

	wc := windows.WndClassEx{
		Size:        uint32(unsafe.Sizeof(windows.WndClassEx{})),
		LpfnWndProc: windows.NewCallback(wndProc),
		Instance:    0,
		LpszClassName: className,
	}

	windows.RegisterClassEx(&wc)

	hwnd, _ := windows.CreateWindowEx(
		0, className, windowName,
		windows.WS_OVERLAPPEDWINDOW|windows.WS_VISIBLE,
		100, 100, 400, 200,
		0, 0, 0, nil,
	)

	// Background ticker to trigger the hang
	go func() {
		for {
			time.Sleep(5 * time.Second)
			
			// Update title before hanging
			title, _ := windows.UTF16PtrFromString("STATUS: !! HUNG !!")
			procSetWindowText.Call(uintptr(hwnd), uintptr(unsafe.Pointer(title)))
			procUpdateWindow.Call(uintptr(hwnd)) // Force redraw so you see the change
			
			fmt.Println("Hanging now for 3 seconds...")
			
			// This is the "bad" part: we send a message to the UI thread 
			// that tells it to sleep, effectively killing the message loop.
			windows.SendMessage(hwnd, 0x8001, 0, 0) 
		}
	}()

	var msg windows.Msg
	for {
		if ret, _ := windows.GetMessage(&msg, 0, 0, 0); ret > 0 {
			windows.TranslateMessage(&msg)
			windows.DispatchMessage(&msg)
		} else {
			break
		}
	}
}

func wndProc(hwnd windows.Handle, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case 0x8001: // Our custom "Hang" message
		time.Sleep(3 * time.Second)
		title, _ := windows.UTF16PtrFromString("STATUS: RUNNING")
		procSetWindowText.Call(uintptr(hwnd), uintptr(unsafe.Pointer(title)))
		return 0
	case windows.WM_DESTROY:
		windows.PostQuitMessage(0)
		return 0
	default:
		return windows.DefWindowProc(hwnd, msg, wparam, lparam)
	}
}