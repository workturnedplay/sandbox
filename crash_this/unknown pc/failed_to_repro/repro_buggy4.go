//go:build windows
// +build windows

package main

import (
cryptoRand "crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
  "io"
	"net/http"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

func gcAndStackMover(wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		// heap churn
		_ = make([]byte, 1<<20) // 1 MiB
		// stack churn via recursion
		churnStack(50)
		// runtime.GC()
		// runtime.Gosched()
		time.Sleep(10 * time.Millisecond)
	}
}

func main() {
	fmt.Println("=== DNSbollocks callWithRetry stack-pointer AV reproducer (BUGGY + aggressive GC) ===")
rand.Seed(time.Now().UnixNano())

	var wg sync.WaitGroup
	wg.Add(1)
	go gcAndStackMover(&wg) // GC churn + recursion stack mover
  
// Start HTTP server
	port := 8080
	go startHTTPServer(port)

// Start HTTP clients
	for i := 0; i < 20; i++ {
		go httpClientWorker(port)
	}

	// Launch multiple GC spammer goroutines
	const numSpammers = 4
	wg.Add(numSpammers)
	for g := 0; g < numSpammers; g++ {
		go func(id int) {
			defer wg.Done()
			for {
				// Large allocations to churn heap and trigger GC
				_ = make([]byte, 2<<20) // 2 MiB
				_ = make([]byte, 1<<20) // 1 MiB
				// runtime.GC()
				// runtime.Gosched()
			}
		}(g)
    go func() {
	for {
		churnStack(50)       // deep recursion, moves stack
		_ = make([]byte, 2<<20) // heap churn
		// runtime.GC()
		// runtime.Gosched()
	}
}()
	}

	start := time.Now()
	const iterations = 200_000

	for i := 0; i < iterations; i++ {
		//_, err := getServiceNamesRepro(uint32(i % 10000))
    err := callWithStack(500, uint32(i % 10000))
    
		if err != nil && i%5000 == 0 {
			fmt.Printf("err at iteration %d: %v\n", i, err)
		}

		// Aggressively trigger GC even in main loop
		// if i%1 == 0 { // every iteration
			// runtime.GC()
			// runtime.Gosched()
		// }
	}

	fmt.Printf("Survived %d calls in %v — bug did not trigger this run.\n", iterations, time.Since(start))
	wg.Wait() // keep background alive (won't reach here on crash)
}

func callWithStack(depth int, targetPID uint32) (error) {
	if depth == 0 {
		_,err:=getServiceNamesRepro(targetPID)
		return err
	}
	var dummy [256]byte
	_ = dummy
	return callWithStack(depth-1, targetPID)
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

		errEnum := windows.EnumServicesStatusEx(
			scm,
			windows.SC_ENUM_PROCESS_INFO,
			windows.SERVICE_WIN32,
			windows.SERVICE_STATE_ALL,
			bufPtr,
			*sizePtr,
			sizePtr, // ← stack-derived pointer from callWithRetryRepro
			&servicesReturned,
			&resume,
			nil,
		)
		return errEnum
	})
	if err != nil {
		return nil, err
	}
	_ = buffer
	return nil, nil
}

// The original presumably-buggy callWithRetryRepro
func callWithRetryRepro(who string, initialSize uint32, call func(bufPtr *byte, sizePtr *uint32) error) ([]byte, error) {
	size := initialSize // STACK variable
	const MAX_RETRIES = 10

	for tries := 0; tries < MAX_RETRIES; tries++ {
		var buf []byte
		var ptr *byte
		if size > 0 {
			buf = make([]byte, size)
			ptr = &buf[0]
		}

		err := call(ptr, &size) // ← &size on stack → passed to Win32 API

		runtime.KeepAlive(ptr)
		runtime.KeepAlive(&size) // too late for call, but keeps Go from GC’ing after call

		if err == nil {
			if uint64(size) > uint64(len(buf)) {
				panic("impossible")
			}
			return buf, nil
		}

		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) &&
			!errors.Is(err, windows.ERROR_MORE_DATA) {
			return nil, err
		}

		// Increase size gradually to handle ERROR_MORE_DATA
		if uint64(size) <= uint64(len(buf)) {
			const increment = 1024
			if math.MaxUint32-size < increment {
				return nil, fmt.Errorf("overflow")
			}
			size += increment
		}
	}

	return nil, fmt.Errorf("retries exceeded")
}

func churnStack(depth int) {
	if depth == 0 {
		return
	}
	var scratch [512]byte // occupy stack space
	_ = scratch
	churnStack(depth - 1)
}

// --- HTTP server ---
func startHTTPServer(port int) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := make([]byte, 2048) // 2 KiB
		if _, err := cryptoRand.Read(data); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(data)
	})
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		panic(err)
	}
	http.Serve(listener, nil)
}

// --- HTTP client ---
func httpClientWorker(port int) {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	for {
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
		if err == nil {
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		time.Sleep(time.Duration(rand.Intn(5)+1) * time.Millisecond)
	}
}
