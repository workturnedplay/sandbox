package main

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"unsafe"

	"golang.org/x/sys/windows"
)

type BoundProc struct {
	Proc  LazyProcish
	Check WinCheckFunc
}

//XXX: crashes without this, doh:
//go:uintptrescapes
func (b *BoundProc) Call(args ...uintptr) (uintptr, uintptr, error) {
	return WinCall(b.Proc, b.Check, args...)
}

type WinCheckFunc func(r1 uintptr) bool

func NewBoundProc(dll *windows.LazyDLL, name string, check WinCheckFunc) *BoundProc {

	if check == nil {
		panic("NewBoundProc: nil WinCheckFunc passed as arg")
	}

	return &BoundProc{
		Proc:  RealProc2(dll, name),
		Check: check,
	}
}

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
	// Name returns the name of the procedure (used in error messages).
	//Why Name() instead of a field? Because interfaces in Go cannot require fields — only methods
	Name() string

	// Call invokes the Windows procedure with the given arguments.
	// Signature must match windows.LazyProc.Call exactly.
	Call(a ...uintptr) (r1, r2 uintptr, lastErr error)
}

// realLazyProc wraps *windows.LazyProc to satisfy LazyProcish.
//
// Embedding gives us .Call() for free via promotion.
type realLazyProc struct {
	*windows.LazyProc
}

// Name implements LazyProcish.
//
// Returns the procedure name for use in error messages.
func (r *realLazyProc) Name() string {
	return r.LazyProc.Name
}

func RealProc(p *windows.LazyProc) LazyProcish {
	return &realLazyProc{LazyProc: p}
}

func RealProc2(dll *windows.LazyDLL, name string) LazyProcish {
	if dll == nil {
		panic("RealProc2: nil dll")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		panic("RealProc2: empty proc name")
	}
	return RealProc(dll.NewProc(name))
}

func CheckWinResult(
	//can be empty
	operationNameToIncludeInErrorMessages string,
	isFailure WinCheckFunc,
	//onFail func(err error),
	r1 uintptr,
	callErr error,
) error {
	if !isFailure(r1) {
		// Success: return nil so 'if err != nil' behaves normally.
		return nil
	}

	// Normalize callErr: treat ERROR_SUCCESS as nil
	if callErr != nil && errors.Is(callErr, windows.ERROR_SUCCESS) {
		callErr = nil
	}

	// If callErr is missing/useless, try to recover from r1
	if callErr == nil {
		// Many Win32 APIs (e.g. GetExtendedUdpTable) return the error in r1.
		// Only treat r1 as an errno if it's non-zero.
		if r1 != 0 {
			errno := windows.Errno(r1)

			// Defensive: avoid ever wrapping ERROR_SUCCESS
			if !errors.Is(errno, windows.ERROR_SUCCESS) {
				// since r1 != 0 already, this is bound to never be ERROR_SUCCESS here, unless r1 != 0 can ever be ERROR_SUCCESS, unsure.
				return fmt.Errorf("%q windows call failed with error: %w", operationNameToIncludeInErrorMessages, errno)
			}
		}

		// Fallback: truly unknown failure
		return fmt.Errorf(
			"%q windows call reported failure (ret=%d) but no usable error was provided",
			operationNameToIncludeInErrorMessages,
			r1,
		)
	}

	// Normal path: we have a meaningful callErr
	return fmt.Errorf("%q windows call failed with error: %w", operationNameToIncludeInErrorMessages, callErr)

}

//XXX: hmm, doesn't crash without this, I guess this means, yes, it's transitive!
//go:uintptrescapes
func WinCall(proc LazyProcish, check WinCheckFunc, args ...uintptr) (uintptr, uintptr, error) {
	if proc == nil {
		panic(fmt.Errorf("WinCall: nil proc"))
	}

	op := strings.TrimSpace(proc.Name())
	if op == "" {
		op = "UnspecifiedWinApi"
	}

	// churn memory before the call
	churn()
	stackChurn(64) // grow stack
	runtime.GC()   // encourage shrink afterwards
	runtime.Gosched()
	smashStack()

	// args is a []uintptr, but because of //go:uintptrescapes, the caller
	// has already pinned the memory safely before we get here.
	r1, r2, callErr := proc.Call(args...)
	err := CheckWinResult(op, check, r1, callErr)
	return r1, r2, err
}

// ---- Test harness ----

var sink any

func churn() {
	// Force GC + stack pressure
	for i := 0; i < 100; i++ {
		b := make([]byte, 1<<20) // 1MB
		sink = b
	}
	//runtime.GC()
}

func stackChurn(depth int) {
	if depth == 0 {
		return
	}

	// Force a large stack frame (~8KB)
	var buf [8192]byte
	buf[0] = byte(depth) // prevent optimization

	stackChurn(depth - 1)

	// use again to avoid dead-store elimination
	if buf[0] == 255 {
		panic("impossible")
	}
}

func main() {
	//c := &realCaller{}

	for i := 0; i < 1_000_000; i++ {
		//var x uint64 = 0x1122334455667788
		f := frame{
			x: 0xDEADBEEFCAFEBABE, // sentinel
		}

		// pointer -> uintptr
		//p := uintptr(unsafe.Pointer(&f.x)) // XXX: yes this crashes if here (normal, doh!)

	// // churn memory before the call
  // //XXX: if you do these here instread of in WinCall, then it may actually crash
	// churn()
	// stackChurn(64) // grow stack
	// runtime.GC()   // encourage shrink afterwards
	// runtime.Gosched()
	// smashStack()


		// call through interface
		procGetSystemTimeAsFileTime.Call(
			//p, //XXX: crashes
			uintptr(unsafe.Pointer(&f.x)), // XXX: doesn't crash!
		)

		if f.x == 0xDEADBEEFCAFEBABE {
			panic("write did NOT land in f.x (stale pointer)")
		}

		if i%1000 == 0 {
			fmt.Println("ok", i)
		}
	}
}

var procGetSystemTimeAsFileTime = NewBoundProc(kernel32, "GetSystemTimeAsFileTime", CheckErrno)
var kernel32 = windows.NewLazySystemDLL("kernel32.dll")

func smashStack() {
	var big [65536]byte
	for i := range big {
		big[i] = 0xCC
	}
}

type frame struct {
	pre  uint64
	x    uint64 // technically should be: struct { LowDateTime, HighDateTime uint32 } but we don't care about using it
	post uint64
}
