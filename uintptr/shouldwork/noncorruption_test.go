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
// simulateSafeSyscall represents the CORRECT way.
// Notice it takes a Pointer, NOT a uintptr.
func simulateSafeSyscall(ptr unsafe.Pointer) {
	// The conversion to uintptr happens at the last possible second.
	// In a real Windows app, this would be: proc.Call(uintptr(ptr))
	addr := uintptr(ptr) 
	*(*uint64)(unsafe.Pointer(addr)) = 0xDEADBEEFCAFEBABE
}

func TestManualUintptrSafe(t *testing.T) {
	// 1. Create target on stack
	targets := make([]uint64, 1)

	// 2. We do NOT store a uintptr here. 
	// We keep the reference as a standard Go pointer (or slice).
	
	stackGrower(1000, func() {
		runtime.GC()

		// 3. PASS THE POINTER.
		// Because we pass the pointer itself into the function, 
		// if the stack moves, the Go Runtime sees this pointer 
		// and UPDATES it to the new memory address automatically.
		simulateSafeSyscall(unsafe.Pointer(&targets[0]))
	})

	// 4. This will ALWAYS pass.
	if targets[0] != 0xDEADBEEFCAFEBABE {
		t.Errorf("This should be impossible in the safe version!")
	} else {
		t.Log("Successfully updated: Go tracked the pointer during stack moves.")
	}
}