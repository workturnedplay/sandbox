package unsafeptr_corruption

import (
	"runtime"
	"testing"
	"unsafe"
)


func TestGCUnsafe_Fail(t *testing.T) {
	buf := make([]byte, 8)

	addr := uintptr(unsafe.Pointer(&buf[0]))

	// Drop the reference early (compiler may treat buf as dead after this point)
	buf2 := buf
	buf = nil

	runtime.GC()

	simulateSyscall(addr)

	// buf2 is never used → compiler may consider underlying memory dead
	if *(*uint64)(unsafe.Pointer(addr)) != 0xDEADBEEFCAFEBABE {
		t.Fatalf("write failed or corrupted")
	}
}


//go:uintptrescapes
// simulateSyscall simulates a Windows API writing 8 bytes to a pointer.
func simulateSyscall(addr uintptr) {
  //runtime.GC()
  	stackGrower(100, func() {
		// 4. Trigger GC for good measure to ensure the old memory might be reclaimed
		runtime.GC()

		// 5. "Call" our fake Windows API with the stale address
		//simulateSyscall(staleAddress)
    //simulateSyscall(uintptr(unsafe.Pointer(&target)))
    // We treat the integer 'addr' as a pointer to a uint64 and write to it.
    // This is exactly what the Windows Kernel does.
    *(*uint64)(unsafe.Pointer(addr)) = 0xDEADBEEFCAFEBABE
	})
	
}

// stackGrower is a recursive function designed to force the Go stack to grow and move.
func stackGrower(n int, fn func()) {
	var dummy [1024]byte // Use some stack space
	_ = dummy
	if n > 0 {
		stackGrower(n-1, fn)
	} else {
		fn()
	}
}