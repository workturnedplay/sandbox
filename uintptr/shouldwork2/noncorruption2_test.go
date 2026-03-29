package unsafeptr_corruption

import (
	"runtime"
	"testing"
	"unsafe"
)
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


// Safe version: We use a pointer to a struct or a direct variable
func simulateSafeSyscall(ptr unsafe.Pointer) {
	// Inside here, ptr is a valid Go pointer. 
	// If the stack moved during the call to this function, 
	// Go updated the value of 'ptr' for us.
	
	addr := uintptr(ptr)
	*(*uint64)(unsafe.Pointer(addr)) = 0xDEADBEEFCAFEBABE
}

func TestManualUintptrSafe2(t *testing.T) {
	// Use a simple variable, not a slice, to avoid slice-header confusion
	var target uint64 = 0

	stackGrower(1000, func() {
		runtime.GC()

		// CORRECT: Passing the pointer to the variable.
		// Go tracks this 'unsafe.Pointer' through the recursion.
		simulateSafeSyscall(unsafe.Pointer(&target))
	})

	if target != 0xDEADBEEFCAFEBABE {
		t.Errorf("The variable was not updated!")
	} else {
		t.Log("Safe test passed: Pointer was tracked correctly.")
	}
}