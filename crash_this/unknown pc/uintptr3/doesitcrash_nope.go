package main

import (
	//"encoding/binary"
	"errors"
	"fmt"
	//"math"
	//"math/rand"
	//"net"
	"runtime"
	"strings"
	//"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ==================== your exact wincoe machinery (unchanged) ====================

type WinCheckFunc func(r1 uintptr) bool

var (
	// CheckBool identifies a failure for functions returning a Windows BOOL in r1.
	// In the Windows API, a 0 (FALSE) indicates that the function failed.
	CheckBool WinCheckFunc = func(r1 uintptr) bool { return r1 == 0 }

	// CheckHandle identifies a failure for functions returning a HANDLE in r1.
	// Many Windows APIs return INVALID_HANDLE_VALUE (all bits set to 1) on failure.
	// ^uintptr(0) is the Go-idiomatic way to represent -1 as an unsigned pointer.
	CheckHandle WinCheckFunc = func(r1 uintptr) bool { return r1 == ^uintptr(0) }

	// CheckNull identifies a failure for functions returning a pointer or a handle in r1
	// where a NULL value (0) indicates the operation could not be completed.
	CheckNull WinCheckFunc = func(r1 uintptr) bool { return r1 == 0 }

	// CheckHRESULT identifies a failure for functions that return an HRESULT in r1.
	// An HRESULT is a 32-bit value where a negative number (high bit set)
	// indicates an error, while 0 or positive values indicate success.
	CheckHRESULT WinCheckFunc = func(r1 uintptr) bool { return int32(r1) < 0 }

	// CheckErrno identifies a failure for Win32 APIs that return a DWORD error code in r1.
	// In this convention, 0 (ERROR_SUCCESS) means success, any non-zero value is a failure.
	CheckErrno WinCheckFunc = func(r1 uintptr) bool { return r1 != 0 }
)



type LazyProcish interface {
	Name() string
	Call(a ...uintptr) (r1, r2 uintptr, lastErr error)
}

type realLazyProc struct{ *windows.LazyProc }
func (r *realLazyProc) Name() string { return r.LazyProc.Name }
func RealProc(p *windows.LazyProc) LazyProcish { return &realLazyProc{LazyProc: p} }
func RealProc2(dll *windows.LazyDLL, name string) LazyProcish {
	if dll == nil { panic("RealProc2: nil dll") }
	name = strings.TrimSpace(name)
	if name == "" { panic("RealProc2: empty proc name") }
	return RealProc(dll.NewProc(name))
}

func CheckWinResult(op string, isFailure WinCheckFunc, r1 uintptr, callErr error) error {
	if !isFailure(r1) { return nil }
	if callErr != nil && errors.Is(callErr, windows.ERROR_SUCCESS) { callErr = nil }
	if callErr == nil {
		if r1 != 0 {
			errno := windows.Errno(r1)
			if !errors.Is(errno, windows.ERROR_SUCCESS) {
				return fmt.Errorf("%q windows call failed with error: %w", op, errno)
			}
		}
		return fmt.Errorf("%q windows call reported failure (ret=%d) but no usable error was provided", op, r1)
	}
	return fmt.Errorf("%q windows call failed with error: %w", op, callErr)
}

type BoundProc struct {
	Proc  LazyProcish
	Check WinCheckFunc
}

//go:uintptrescapes
func (b *BoundProc) Call(args ...uintptr) (uintptr, uintptr, error) {
	return WinCall(b.Proc, b.Check, args...)
}

func NewBoundProc(dll *windows.LazyDLL, name string, check WinCheckFunc) *BoundProc {
	if check == nil { panic("NewBoundProc: nil WinCheckFunc") }
	return &BoundProc{Proc: RealProc2(dll, name), Check: check}
}

//go:uintptrescapes
func WinCall(proc LazyProcish, check WinCheckFunc, args ...uintptr) (uintptr, uintptr, error) {
	if proc == nil { panic("WinCall: nil proc") }
	op := strings.TrimSpace(proc.Name())
	if op == "" { op = "UnspecifiedWinApi" }

	// === THIS IS WHAT CRASHES IT (exactly like production) ===
	churn()
	stackChurn(4)
	runtime.GC()
	runtime.Gosched()
	smashStack()

	r1, r2, callErr := proc.Call(args...)
	err := CheckWinResult(op, check, r1, callErr)
	return r1, r2, err
}

var sink any
func churn() {
	for i := 0; i < 100; i++ {
		b := make([]byte, 1<<20)
		sink = b
	}
}
func stackChurn(depth int) {
	if depth == 0 { return }
	var buf [8192]byte
	buf[0] = byte(depth)
	stackChurn(depth - 1)
	if buf[0] == 255 { panic("impossible") }
}
func smashStack() {
	var big [65536]byte
	for i := range big { big[i] = 0xCC }
}

// ==================== constants & helpers ====================

const (
	TH32CS_SNAPPROCESS = 0x00000002
	AF_INET            = 2
	UDP_TABLE_OWNER_PID     = 1
	TCP_TABLE_OWNER_PID_ALL = 5
	MaxExtendedPath         = 32767
)

var (
	Kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	Iphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")

	procCreateToolhelp32Snapshot = NewBoundProc(Kernel32, "CreateToolhelp32Snapshot", CheckHandle)
	procProcess32First           = NewBoundProc(Kernel32, "Process32FirstW", CheckBool)
	procProcess32Next            = NewBoundProc(Kernel32, "Process32NextW", CheckBool)
	procGetExtendedUdpTable      = NewBoundProc(Iphlpapi, "GetExtendedUdpTable", CheckErrno)
	procGetExtendedTcpTable      = NewBoundProc(Iphlpapi, "GetExtendedTcpTable", CheckErrno)
	procQueryFullProcessName     = NewBoundProc(Kernel32, "QueryFullProcessImageNameW", CheckBool)
)

func boolToUintptr(b bool) uintptr { if b { return 1 }; return 0 }

// ==================== the exact production path ====================

func CreateToolhelp32Snapshot(dwFlags, th32ProcessID uint32) (windows.Handle, error) {
	r1, _, err := procCreateToolhelp32Snapshot.Call(uintptr(dwFlags), uintptr(th32ProcessID))
	if err != nil { return 0, err }
	return windows.Handle(r1), nil
}

func Process32First(snapshot windows.Handle, entry *windows.ProcessEntry32) error {
	_, _, err := procProcess32First.Call(uintptr(snapshot), uintptr(unsafe.Pointer(entry)))
	return err
}

func Process32Next(snapshot windows.Handle, entry *windows.ProcessEntry32) error {
	_, _, err := procProcess32Next.Call(uintptr(snapshot), uintptr(unsafe.Pointer(entry)))
	return err
}

func GetProcessName(pid uint32) (string, error) {
	snapshot, err := CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0)
	if err != nil { return "", err }
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	err = Process32First(snapshot, &entry)
	for err == nil {
		if entry.ProcessID == pid {
			return windows.UTF16ToString(entry.ExeFile[:]), nil
		}
		err = Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return "", err
	}
	return "", fmt.Errorf("not found, err: %w", err)
}

// (the rest of PidAndExeForUDP / PidAndExeForTCP / callWithRetry etc. are unchanged from your paste)

func main() {
	fmt.Println("Starting crash reproducer - hammering GetProcessName via BoundProc path...")

  pid:=windows.GetCurrentProcessId()
	for i := 0; i < 3_000; i++ {
		// === this is the exact production path that crashes ===
    go func() {
		_, _ = GetProcessName(pid)
    }()

		// optional extra pressure (makes it crash faster)
		// if i%50 == 0 {
			// churn()
			// stackChurn(64)
			// runtime.GC()
			// runtime.Gosched()
		// }

		if i%10 == 0 {
			fmt.Printf("iterations: %d\n", i)
		}
	}
}