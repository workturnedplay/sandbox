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
	procCallNextHookEx    = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHook = user32.NewProc("UnhookWindowsHookEx")
	procGetMessage        = user32.NewProc("GetMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessage   = user32.NewProc("DispatchMessageW")
	procEnumWindows       = user32.NewProc("EnumWindows")
	procGetWindowText     = user32.NewProc("GetWindowTextW")
	procGetWindowThread   = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowLongPtr  = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr  = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProc    = user32.NewProc("CallWindowProcW")

	hook            windows.Handle
	originalWndProc uintptr
	targetHwnd      windows.Handle
)

// Manual POINT definition (safe and self-contained)
type POINT struct {
	X, Y int32
}

type MSG struct {
	Hwnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type CWPSTRUCT struct {
	LParam  uintptr
	WParam  uintptr
	Message uint32
	Hwnd    windows.Handle
}

// const GWL_WNDPROC = -4
// const GWL_WNDPROC = uintptr(0xFFFFFFFFFFFFFFFC) // -4 as 64-bit two's complement
// or more clearly:
const GWL_WNDPROC = ^uintptr(3) // bitwise NOT of 3 = 0x...FFFC = -4 on 64-bit

// Your subclass callback — logs every message that reaches the target's WndProc
func subclassCallback(hwnd windows.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	// Log the message (you can filter here if you want less noise)
	fmt.Printf("Msg: 0x%04X, wParam: 0x%08X, lParam: 0x%016X\n", msg, wParam, lParam)

	// Optional: Force drag by returning HTCAPTION on hit-test
	// Uncomment the next 3 lines if you want to test forced drag behavior
	// if msg == 0x0084 { // WM_NCHITTEST
	//     return 2 // HTCAPTION
	// }

	// Forward to original window procedure
	ret, _, _ := procCallWindowProc.Call(
		originalWndProc,
		uintptr(hwnd),
		uintptr(msg),
		wParam,
		lParam,
	)
	return ret
}

// Subclass the target window
func subclassTarget(target windows.Handle) error {
	cb := syscall.NewCallback(subclassCallback)

	// Get original WndProc
	orig, _, err := procGetWindowLongPtr.Call(
		uintptr(target),
		uintptr(GWL_WNDPROC),
	)
	if orig == 0 {
		return fmt.Errorf("GetWindowLongPtr failed: %v", err)
	}
	originalWndProc = orig

	// Replace with our callback
	_, _, err = procSetWindowLongPtr.Call(
		uintptr(target),
		GWL_WNDPROC,
		cb,
	)
	if err != syscall.Errno(0) {
		return fmt.Errorf("SetWindowLongPtr failed: %v", err)
	}

	fmt.Printf("Successfully subclassed HWND 0x%X\n", target)
	return nil
}

// Restore original WndProc
func unsubclassTarget(target windows.Handle) {
	if originalWndProc == 0 {
		return
	}
	procSetWindowLongPtr.Call(
		uintptr(target),
		GWL_WNDPROC,
		originalWndProc,
	)
	fmt.Printf("Restored original WndProc for HWND 0x%X\n", target)
	originalWndProc = 0
}

func enumCallback(hwnd uintptr, lParam uintptr) uintptr {
	var buf [256]uint16
	procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	title := windows.UTF16ToString(buf[:])
	if title != "" {
		fmt.Printf("HWND: 0x%08X  Title: %s\n", hwnd, title)
	}
	return 1 // continue enumeration
}

func main() {
	runtime.LockOSThread() // Recommended for message loops / callbacks

	// Enumerate top-level windows
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
	targetHwnd = windows.Handle(hwnd64)

	// Subclass the target
	if err := subclassTarget(targetHwnd); err != nil {
		fmt.Printf("Subclassing failed: %v\n", err)
		return
	}

	// Make sure we clean up on exit
	defer unsubclassTarget(targetHwnd)

	// Graceful shutdown on Ctrl+C
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\nShutting down...")
		unsubclassTarget(targetHwnd)
		os.Exit(0)
	}()

	fmt.Printf("Spying on HWND 0x%X... Interact with the window. Ctrl+C to stop.\n", targetHwnd)

	// Message pump to keep the program alive
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
