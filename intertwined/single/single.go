package main

import (
	"fmt"
	"runtime"

	"golang.org/x/sys/windows"
)

var (
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procSetLastError = kernel32.NewProc("SetLastError")
	procGetLastError = kernel32.NewProc("GetLastError")
)

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
  
  // Ensures GetProcAddress doesn't run and ruin our LastError state later.
  _ = procSetLastError.Addr()
  _ = procGetLastError.Addr()

	for id := 1; id <= 50; id++ {
		magic := uint32(id)

//XXX: useless to set/get lasterr because each LazyProc.Call() does set it to 0, calls the syscall/api, gets it and returns it in err(3rd arg); all 3 are atomic, unpreemtible.
		// Set
		_, _, _ = procSetLastError.Call(uintptr(magic))

		// Get immediately
		r1, _, callErr := procGetLastError.Call() // ZERO arguments!

		fmt.Printf("Set(%d) -> r1=%d, callErr=%v\n", magic, r1, callErr)

		if r1 == 0 {
			panic(fmt.Sprintf("FAILED: Set(%d) but Get r1==0", magic))
		}

		if uint32(r1) != magic {
			fmt.Printf("MISMATCH: got %d, wanted %d\n", r1, magic)
		}
	}
	fmt.Println("Single-thread locked test passed.")
}
