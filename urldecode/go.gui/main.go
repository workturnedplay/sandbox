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
	modGdi32    = syscall.NewLazyDLL("gdi32.dll")
	modDwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	modUxtheme  = syscall.NewLazyDLL("uxtheme.dll")

	procRegisterClassExW     = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW      = modUser32.NewProc("CreateWindowExW")
	procDefWindowProcW       = modUser32.NewProc("DefWindowProcW")
	procGetMessageW          = modUser32.NewProc("GetMessageW")
	procTranslateMessage     = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW     = modUser32.NewProc("DispatchMessageW")
	procPostQuitMessage      = modUser32.NewProc("PostQuitMessage")
	procGetWindowTextLengthW = modUser32.NewProc("GetWindowTextLengthW")
	procGetWindowTextW       = modUser32.NewProc("GetWindowTextW")
	procSetWindowTextW       = modUser32.NewProc("SetWindowTextW")
	procSetFocus             = modUser32.NewProc("SetFocus")
	procSendMessageW         = modUser32.NewProc("SendMessageW")
	procSetWindowLongPtrW    = modUser32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW      = modUser32.NewProc("CallWindowProcW")
	procGetKeyState          = modUser32.NewProc("GetKeyState")
	procGetModuleHandleW     = modKernel32.NewProc("GetModuleHandleW")

	procCreateSolidBrush     = modGdi32.NewProc("CreateSolidBrush")
	procSetTextColor         = modGdi32.NewProc("SetTextColor")
	procSetBkColor           = modGdi32.NewProc("SetBkColor")
	procDeleteObject         = modGdi32.NewProc("DeleteObject")

	procDwmSetWindowAttribute = modDwmapi.NewProc("DwmSetWindowAttribute")
	procSetWindowTheme        = modUxtheme.NewProc("SetWindowTheme")
)

const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_VSCROLL          = 0x00200000
	WS_BORDER           = 0x00800000
	ES_MULTILINE        = 0x0004
	ES_AUTOVSCROLL      = 0x0040

	WM_DESTROY          = 0x0002
	WM_COMMAND          = 0x0111
	WM_KEYDOWN          = 0x0100
	WM_CTLCOLOREDIT     = 0x0133
	WM_CTLCOLORSTATIC   = 0x0138
	EN_CHANGE           = 0x0300
	EM_SETSEL           = 0x00B1

	GWLP_WNDPROC        = -4
	VK_CONTROL          = 0x11
	
	DWMWA_USE_IMMERSIVE_DARK_MODE = 20

	ID_EDIT_INPUT  = 101
	ID_EDIT_OUTPUT = 102
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

var (
	hEditIn      uintptr
	hEditOut     uintptr
	origEditProc uintptr

	// Dark Theme GDI Resources (RGB -> 0x00BBGGRR)
	textColor   uint32 = 0x00DCDCDC // Off-white #DCDCDC
	bkColor     uint32 = 0x00282828 // Dark Gray #282828
	winBgColor  uint32 = 0x001E1E1E // Main Window #1E1E1E

	hBgBrush   uintptr
	hEditBrush uintptr
)

// editSubclassProc handles custom key messages for the EDIT controls (e.g., Ctrl+A).
func editSubclassProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	if msg == WM_KEYDOWN && wParam == 'A' {
		// Check if Ctrl key is pressed
		res, _, _ := procGetKeyState.Call(VK_CONTROL)
		if int16(res) < 0 {
			// Send EM_SETSEL with 0 and -1 to select all text in the edit control
			procSendMessageW.Call(hwnd, EM_SETSEL, 0, ^uintptr(0))
			return 0 // Swallow key event to prevent system error beep
		}
	}
	r, _, _ := procCallWindowProcW.Call(origEditProc, hwnd, uintptr(msg), wParam, lParam)
	return r
}

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		if uint16(wParam>>16) == EN_CHANGE && lParam == hEditIn {
			updateDecode()
		}
		return 0

	case WM_CTLCOLOREDIT, WM_CTLCOLORSTATIC:
		hdc := wParam
		procSetTextColor.Call(hdc, uintptr(textColor))
		procSetBkColor.Call(hdc, uintptr(bkColor))
		return hEditBrush

	case WM_DESTROY:
		procDeleteObject.Call(hBgBrush)
		procDeleteObject.Call(hEditBrush)
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
	className, _ := syscall.UTF16PtrFromString("URLDecoderDarkClass")
	windowTitle, _ := syscall.UTF16PtrFromString("URL Decoder")
	editClass, _ := syscall.UTF16PtrFromString("EDIT")

	hBgBrush, _, _ = procCreateSolidBrush.Call(uintptr(winBgColor))
	hEditBrush, _, _ = procCreateSolidBrush.Call(uintptr(bkColor))

	var wc WNDCLASSEXW
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = syscall.NewCallback(wndProc)
	wc.HInstance = hInstance
	wc.LpszClassName = className
	wc.HbrBackground = hBgBrush

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		WS_OVERLAPPEDWINDOW|WS_VISIBLE,
		100, 100, 640, 480,
		0, 0, hInstance, 0,
	)

	// Force Windows 11 Immersive Dark Mode on the Title Bar
	darkMode := int32(1)
	procDwmSetWindowAttribute.Call(
		hwnd,
		DWMWA_USE_IMMERSIVE_DARK_MODE,
		uintptr(unsafe.Pointer(&darkMode)),
		uintptr(unsafe.Sizeof(darkMode)),
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

	// Apply dark scrollbars via UxTheme
	darkThemeName, _ := syscall.UTF16PtrFromString("DarkMode_Explorer")
	procSetWindowTheme.Call(hEditIn, uintptr(unsafe.Pointer(darkThemeName)), 0)
	procSetWindowTheme.Call(hEditOut, uintptr(unsafe.Pointer(darkThemeName)), 0)

	// Subclass Edit controls to enable Ctrl+A support
	subclassCb := syscall.NewCallback(editSubclassProc)
	origEditProc, _, _ = procSetWindowLongPtrW.Call(hEditIn, ^uintptr(3), subclassCb)
	procSetWindowLongPtrW.Call(hEditOut, ^uintptr(3), subclassCb)

	procSetFocus.Call(hEditIn)

	var msg MSG
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}