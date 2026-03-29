// doesn't do what it's meant to do, thanks chatgpt /s
package unsafeptr_corruption

import (
	"runtime"
	"testing"
	"unsafe"
  "fmt"
)

var sink any // prevent optimization

func hammerAlloc() {
	for i := 0; i < 1_000_000; i++ {
		b := make([]byte, 1024)
		sink = b
	}
}

func churn() {
	for i := 0; i < 1_000; i++ {
		b := make([]byte, 1_024_000_000)
		sink = b
    if i % 100 == 0 {
      fmt.Printf("%d\n",i)
    }
	}
}


func TestGCUnsafe_Fail(t *testing.T) {
	buf := make([]byte, 8)

	addr := uintptr(unsafe.Pointer(&buf[0]))

  // Make buf "dead" from compiler perspective
	buf = nil

	// runtime.GC()
  // hammerAlloc() // force reuse
// Force GC + heavy allocator churn
	runtime.GC()
	churn()
	runtime.GC()

	// Now write through stale address
	simulateSyscall(addr)

	// Only use uintptr-based access (no Go pointer resurrection)
	val := *(*uint64)(unsafe.Pointer(addr))
	//sink = val

	if val != 0xDEADBEEFCAFEBABE {
		t.Fatalf("FAILED: got %#x", val)
	}
}

func TestKeepAliveMatters(t *testing.T) {
	buf := make([]byte, 8)

	addr := uintptr(unsafe.Pointer(&buf[0]))

	done := false

	go func() {
		runtime.GC()
		done = true
	}()

	for !done {
	}

	// Without KeepAlive, compiler is allowed to treat buf as dead earlier
	simulateSyscall(addr)

	runtime.KeepAlive(buf)

	val := *(*uint64)(unsafe.Pointer(addr))
	if val != 0xDEADBEEFCAFEBABE {
		t.Fatalf("unexpected: %#x", val)
	}
}


//NOgo:uintptrescapes
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