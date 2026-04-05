//go:build windows
// +build windows

package main

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

func main() {
	fmt.Println("=== DNSbollocks callWithRetry stack-pointer AV reproducer (BUGGY + aggressive GC) ===")
	fmt.Println("Background GC spammer running. Expect 0xc0000005 very soon.")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			// Heavy allocation + GC pressure
			_ = make([]byte, 1<<20) // 1 MiB
			runtime.GC()
			runtime.Gosched()
			time.Sleep(10 * time.Millisecond)
		}
	}()

	start := time.Now()
	for i := 0; i < 200_000; i++ {
		_, err := getServiceNamesRepro(uint32(i % 10000))
		if err != nil && i%5000 == 0 {
			fmt.Printf("err at iteration %d: %v\n", i, err)
		}

		if i%20 == 0 {
			runtime.GC()
			runtime.Gosched()
		}
	}

	fmt.Printf("Survived 200k calls in %v — bug did not trigger this run.\n", time.Since(start))
	wg.Wait() // keep background alive (won't reach here on crash)
}

func getServiceNamesRepro(targetPID uint32) ([]string, error) {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return nil, fmt.Errorf("OpenSCManager: %w", err)
	}
	defer windows.CloseServiceHandle(scm)

	buffer, err := callWithRetryRepro("repro", 0, func(bufPtr *byte, sizePtr *uint32) error {
		var servicesReturned uint32
		var resume uint32

		fmt.Printf("!before5\n")
		errEnum := windows.EnumServicesStatusEx(
			scm,
			windows.SC_ENUM_PROCESS_INFO,
			windows.SERVICE_WIN32,
			windows.SERVICE_STATE_ALL,
			bufPtr,
			*sizePtr,
			sizePtr,                    // ← stack-derived pointer from callWithRetryRepro
			&servicesReturned,
			&resume,
			nil,
		)
		fmt.Printf("!after5\n")
		return errEnum
	})
	if err != nil {
		return nil, err
	}
	_ = buffer
	return nil, nil
}

// Your current (buggy) callWithRetry — size lives on the stack
func callWithRetryRepro(who string, initialSize uint32, call func(bufPtr *byte, sizePtr *uint32) error) ([]byte, error) {
	size := initialSize                     // ← STACK variable
	const MAX_RETRIES = 10

	for tries := 0; tries < MAX_RETRIES; tries++ {
		fmt.Printf("!%s before6 try %d, initialSize=%d size=%d\n", who, tries, initialSize, size)

		var buf []byte
		var ptr *byte
		if size > 0 {
			buf = make([]byte, size)
			ptr = &buf[0]
			fmt.Printf("!%s middle7(created buf) try %d, buf=%p ptr=%p size=%d len=%d\n",
				who, tries, buf, ptr, size, len(buf))
		}

		fmt.Printf("!%s before7 try %d, ptr=%p &size=%p size=%d\n", who, tries, ptr, &size, size)

		err := call(ptr, &size)   // ← &size computed on stack → passed through closure

		runtime.KeepAlive(ptr)
		runtime.KeepAlive(&size)  // too late — pointer already escaped

		fmt.Printf("!%s after7 try %d, ptr=%p &size=%p size=%d\n", who, tries, ptr, &size, size)

		if err == nil {
			fmt.Printf("!%s middle7(ret ok) try %d, buf=%p len=%d size=%d\n", who, tries, buf, len(buf), size)
			if uint64(size) > uint64(len(buf)) {
				panic("impossible")
			}
			return buf, nil
		}

		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) &&
			!errors.Is(err, windows.ERROR_MORE_DATA) {
			return nil, err
		}

		if uint64(size) <= uint64(len(buf)) {
			const increment = 1024
			if math.MaxUint32-size < increment {
				return nil, fmt.Errorf("overflow")
			}
			size += increment
		}
		fmt.Printf("!%s after6(end of for) try %d\n", who, tries)
	}
	return nil, fmt.Errorf("retries exceeded")
}