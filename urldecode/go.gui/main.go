package main

import (
	"net/url"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW  = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW   = modUser32.NewProc("CreateWindowExW")
	procDefWindowProcW    = modUser32.NewProc("DefWindowProcW")
	procGetMessageW       = modUser32.NewProc("GetMessageW")
	procTranslateMessage  = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW  = modUser32.NewProc("DispatchMessageW")
	procPostQuitMessage   = modUser32.NewProc("PostQuitMessage")
	procGetWindowTextLengthW = modUser32.NewProc("GetWindowTextLengthW")
	procGetWindowTextW    = modUser32.NewProc("GetWindowTextW")
	procSetWindowTextW    = modUser32.NewProc("SetWindowTextW")
	procSetFocus          = modUser32.NewProc("SetFocus")
	procGetModuleHandleW  = modKernel32.NewProc("GetModuleHandleW")
)

const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_VSCROLL          = 0x00200000
	ES_MULTILINE        = 0x0004
	ES_AUTOVSCROLL      = 0x0040
	WS_BORDER           = 0x00800000
	WM_DESTROY          = 0x0002
	WM_COMMAND          = 0x0111
	EN_CHANGE           = 0x0300
	ID_EDIT_INPUT       = 101
	ID_EDIT_OUTPUT      = 102
)

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

var hEditIn, hEditOut uintptr

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		// Triggered when input text changes (EN_CHANGE notification)
		if uint16(wParam>>16) == EN_CHANGE && lParam == hEditIn {
			updateDecode()
		}
		return 0
	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func updateDecode() {
	lenR, _, _ := procGetWindowTextLengthW.Call(hEditIn)
	if lenR == 0 {
		empty, _ := syscall.UTF16PtrFromString("")
		procSetWindowTextW.Call(hEditOut, uintptr(unsafe.Pointer(empty)))
		return
	}

	buf := make([]uint16, lenR+1)
	procGetWindowTextW.Call(hEditIn, uintptr(unsafe.Pointer(&buf[0])), uintptr(lenR+1))
	inputStr := syscall.UTF16ToString(buf)

	decoded, err := url.QueryUnescape(inputStr)
	if err != nil {
		decoded = "Error: Invalid URL encoding"
	}

	outPtr, _ := syscall.UTF16PtrFromString(decoded)
	procSetWindowTextW.Call(hEditOut, uintptr(unsafe.Pointer(outPtr)))
}

func main() {
	runtime.LockOSThread()

	hInstance, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("URLDecoderClass")
	windowTitle, _ := syscall.UTF16PtrFromString("URL Decoder")
	editClass, _ := syscall.UTF16PtrFromString("EDIT")

	var wc WNDCLASSEXW
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = syscall.NewCallback(wndProc)
	wc.HInstance = hInstance
	wc.LpszClassName = className
	wc.HbrBackground = 6 // COLOR_WINDOW + 1

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		WS_OVERLAPPEDWINDOW|WS_VISIBLE,
		100, 100, 640, 480,
		0, 0, hInstance, 0,
	)

	// Top Edit Box: Encoded Input
	hEditIn, _, _ = procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(editClass)),
		0,
		WS_CHILD|WS_VISIBLE|WS_BORDER|ES_MULTILINE|ES_AUTOVSCROLL|WS_VSCROLL,
		10, 10, 604, 200,
		hwnd, ID_EDIT_INPUT, hInstance, 0,
	)

	// Bottom Edit Box: Decoded Output
	hEditOut, _, _ = procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(editClass)),
		0,
		WS_CHILD|WS_VISIBLE|WS_BORDER|ES_MULTILINE|ES_AUTOVSCROLL|WS_VSCROLL,
		10, 220, 604, 200,
		hwnd, ID_EDIT_OUTPUT, hInstance, 0,
	)

	procSetFocus.Call(hEditIn)

	var msg MSG
	for {
		r := procGetGetMessage(&msg)
		if int32(r) <= 0 { // Handles both WM_QUIT (0) and GetMessage failure (-1)
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func procGetGetMessage(msg *MSG) uintptr {
	r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(msg)), 0, 0, 0)
	return r
}