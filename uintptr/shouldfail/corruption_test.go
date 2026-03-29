package unsafeptr_corruption

import (
	"runtime"
	"testing"
	"unsafe"
)

// simulateSyscall simulates a Windows API writing 8 bytes to a pointer.
func simulateSyscall(addr uintptr) {
	// We treat the integer 'addr' as a pointer to a uint64 and write to it.
	// This is exactly what the Windows Kernel does.
	*(*uint64)(unsafe.Pointer(addr)) = 0xDEADBEEFCAFEBABE
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

func TestUintptrCorruption(t *testing.T) {
	// 1. Create a local variable on the stack.
	var target uint64 = 0
	
	// 2. Capture its address as a uintptr.
	// This is the "Illegal" move your current code makes.
	staleAddress := uintptr(unsafe.Pointer(&target))

	// 3. Force the stack to move. 
	// We call a recursive function that triggers a stack expansion.
	// When the stack expands, 'target' moves to a new memory address,
	// but 'staleAddress' (being just an integer) stays pointing at the old one.
	stackGrower(100, func() {
		// 4. Trigger GC for good measure to ensure the old memory might be reclaimed
		runtime.GC()

		// 5. "Call" our fake Windows API with the stale address
		simulateSyscall(staleAddress)
	})

	// 6. Check if our variable was updated.
	// If the stack moved, 'target' is now at a new address.
	// simulateSyscall wrote to the OLD address (where your HTTP server might now live!)
	if target != 0xDEADBEEFCAFEBABE {
		t.Errorf("CRITICAL: Stale uintptr detected! target was NOT updated. " +
			"The write happened to a memory location that no longer belongs to this variable.")
	} else {
		t.Log("Successfully updated (this time... memory corruption is random!)")
	}
}
