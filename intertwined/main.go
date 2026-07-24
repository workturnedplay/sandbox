package main

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

var kernel32 = windows.NewLazySystemDLL("kernel32.dll")
var procSetLastError = kernel32.NewProc("SetLastError")
var procGetLastError = kernel32.NewProc("GetLastError")

func main() {
	// Ensures GetProcAddress doesn't run and ruin our LastError state later.
	_ = procSetLastError.Addr()
	_ = procGetLastError.Addr()

	// Force the Go runtime to only use 2 OS threads for Go code.
	// This creates massive contention. If multiple goroutines could share
	// a locked thread, we would absolutely see it here.
	runtime.GOMAXPROCS(2)

	var wg sync.WaitGroup
	var raceHits int32

	fmt.Println("Starting brutal race test on LastError...")

	// Spawn 100 goroutines fighting over 2 OS threads
	for i := 1; i <= 1000; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 1. LOCK THE THREAD (The two-way exclusive bind)
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()

			// We loop a few times to increase the chance of a collision
			for j := 0; j < 1000; j++ {
				// 2. Set a magic error code unique to this goroutine
				// (We use id to ensure every goroutine has a different value)
				magicErr := uint32(id)
				//windows.SetLastError(magicErr) // doesn't exist
				_, _, _ = procSetLastError.Call(uintptr(magicErr))
				//so getlasterror is useless: https://github.com/golang/go/issues/41220
				// currentErr1 := windows.GetLastError()
				currentErr1, _, _ := procGetLastError.Call() //same as windows.GetLastError()
				if currentErr1 == 0 {
					panic(fmt.Sprintf("Failed to SetLastError to %d, it was nil aka 0", magicErr)) // hitting this: panic: Failed to SetLastError to 4, it was nil aka 0
				}
				//code1 := uint32(currentErr1.(syscall.Errno)) //panic: interface conversion: error is nil, not syscall.Errno
				code1 := uint32(currentErr1)
				if code1 != magicErr {
					fmt.Printf("Failed to SetLastError to %d", magicErr)
					return
				}
				// 3. THE DANGER ZONE
				// We actively yield the processor. We are begging the Go
				// scheduler to pause us and put another goroutine on this
				// exact OS thread.
				runtime.Gosched()

				// (Optional: add a tiny sleep to really force context switching)
				time.Sleep(1 * time.Millisecond)

				// 4. Check if the error code is still ours
				// If another goroutine ran on this thread while we were yielded,
				// they would have overwritten LastError with THEIR ID.
				currentErr := windows.GetLastError()
				if currentErr == nil {
					continue
				}

				// The underlying error code in Windows
				code := uint32(currentErr.(syscall.Errno))
				if code != magicErr {
					atomic.AddInt32(&raceHits, 1)
					fmt.Printf("RACE HIT! G-%d expected %d but found %d\n", id, magicErr, code)
					return
				}
			}
		}(i)
	}

	wg.Wait()

	if raceHits == 0 {
		fmt.Println("Result: 0 race hits. The TLS state was completely protected.")
	} else {
		fmt.Printf("Result: %d race hits detected!\n", raceHits)
	}
}
