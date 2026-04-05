//fail, no crashes
package main

import (
	"errors"
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"
  "os"
  "golang.org/x/term"
)

func GoRoutineId() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// "goroutine 17 [running]:\n..."
	var id int64 = -1
	fmt.Sscanf(string(buf[:n]), "goroutine %d", &id)
	return id
}

func flush() {
	//fmt.Printf("[GoR:%d] !flushing stderr\n", GoRoutineId())
	os.Stderr.Sync() // Tell Windows to flush the file buffers to disk/console
	//fmt.Printf("[GoR:%d] !flushing stdout\n", GoRoutineId())
	os.Stdout.Sync() // Tell Windows to flush the file buffers to disk/console
}

// The "Attacker": Constantly churns memory and forces GC
func smashyfor() {
	for {
		// // 1. Grow the stack
		// stackChurn(100)
		
		// // 2. Allocate heap memory to trigger GC pressure
		// _ = make([]byte, 1<<20) // 1MB allocation
		
		// // 3. Explicitly call GC and Gosched to move stacks around
		// runtime.GC()
		// //runtime.LogEvent("GC_DONE") // If supported, helps marking
		// runtime.Gosched()
		
		// // 4. Smash local stack memory with "0xe8" (the CALL opcode)
		// smashStack()
    smashy(false)
		
		// Use a tiny, non-aligned sleep to avoid syncing with the TCP loop
		time.Sleep(17 * time.Millisecond)
	}
}

func smashy(log bool) {
	if _, exists := os.LookupEnv("WINCOE_SMASHY_TEST"); exists { // only for deliberate stress testing
		if log {
			fmt.Printf("[GoR:%d] !starting Smashy\n", GoRoutineId())
			fmt.Printf("[GoR:%d] ! starting churn()\n", GoRoutineId())
			flush()
		}
		Churn()
		if log {
			fmt.Printf("[GoR:%d] ! ending churn()\n", GoRoutineId())
			fmt.Printf("[GoR:%d] ! starting stackChurn(64)\n", GoRoutineId())
			flush()
		}

		stackChurn(64) // grow stack
		if log {
			fmt.Printf("[GoR:%d] ! ending stackChurn(64)\n", GoRoutineId())
			flush()
		}

		if _, exists2 := os.LookupEnv("WINCOE_SMASHY_RUNGC"); exists2 {
			if log {
				fmt.Printf("[GoR:%d] ! before runtime.GC()\n", GoRoutineId())
				flush()
			}
			runtime.GC() // encourage shrink afterwards
			if log {
				fmt.Printf("[GoR:%d] ! after runtime.GC()\n", GoRoutineId())
				flush()
			}
			if log {
				fmt.Printf("[GoR:%d] ! before runtime.Gosched()\n", GoRoutineId())
				flush()
			}
			runtime.Gosched()
			if log {
				fmt.Printf("[GoR:%d] ! after runtime.Gosched()\n", GoRoutineId())
				flush()
			}
		} //if

		if log {
			fmt.Printf("[GoR:%d] ! starting smashStack\n", GoRoutineId())
			flush()
		}
		smashStack()
		if log {
			fmt.Printf("[GoR:%d] ! ending smashStack\n", GoRoutineId())
			fmt.Printf("[GoR:%d] !ending Smashy\n", GoRoutineId())
			flush()
		}
	}
}


// func stackChurn(depth int) {
	// if depth <= 0 {
		// return
	// }
	// var dummy [1024]byte // Use some stack space
	// for i := range dummy {
		// dummy[i] = 0xe8
	// }
	// stackChurn(depth - 1)
// }

// func smashStack() {
	// var big [8192]byte
	// for i := range big {
		// big[i] = 0xe8 // Fill with the "ghost" pointer address
	// }
// }

func smashStack() {
	var big [65536]byte
	for i := range big {
		big[i] = 0xCC
	}
}

func Churn() {
	var sink any
	// Force GC + stack pressure
	for i := 0; i < 100; i++ {
		b := make([]byte, 1<<20) // 1MB
		sink = b
	}
	_ = sink
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


// The "Victim": A TCP loop with a very tight deadline
func tcpVictim() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer ln.Close()

	fmt.Printf("TCP Victim listening on %s\n", ln.Addr())

	for {
		// SETTING THE TRAP: Constant churn of the network poller
		// We use 20ms to stay just above the Windows clock tick (15.6ms)
		ln.(*net.TCPListener).SetDeadline(time.Now().Add(41 * time.Millisecond))

		conn, err := ln.Accept()
    smashy(false)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				// This is the moment of vulnerability!
				// The goroutine is waking up from a 'parked' state.
				continue
			}
      panic(fmt.Sprintf("actual err from tcp Accept(), err:%v",err))
			continue
		}
    panic("no err, something connected")
		conn.Close()
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	
	fmt.Println("Starting stress test...")
	fmt.Println("Check for '0xe8' or 'unwinder.next' in the crash output.")

	var wg sync.WaitGroup

	// Start multiple attackers to keep the GC busy Mark-ing
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			smashyfor()
			wg.Done()
		}()
	}

	// Start multiple victims to increase collision probability
	for i := 0; i < 80; i++ {
		wg.Add(1)
		go func() {
			tcpVictim()
			wg.Done()
		}()
	}

	// Progress indicator
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		for range ticker.C {
			fmt.Println("Still running... No crash yet.")
		}
	}()
  
  go watchKeys(
		func() { // Ctrl+R
		},
		func() { // alt+x etc.
			fmt.Println("Shutdown signal received, clean exit.")
      os.Exit(1)
		},
	)

	wg.Wait()
}

func watchKeys(reloadFn func(), cleanExitFn func()) {
	fd := int(os.Stdin.Fd())

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return
	}
	defer term.Restore(fd, oldState)

	buf := make([]byte, 3)

	for {
		fmt.Print(".")
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			continue
		}

		// Ctrl+X (0x18)
		if buf[0] == 0x18 {
			fmt.Println("\nCtrl+X detected → clean exit")
			_ = term.Restore(fd, oldState)
			cleanExitFn()
		}

		// Ctrl+R (0x12)
		if buf[0] == 0x12 {
			fmt.Println("\nCtrl+R detected → reloading config")
			//_ = term.Restore(fd, oldState)
			// NO restore needed here because we want to stay in Raw mode
			// to catch the next keypress after the reload.
			reloadFn()
		}

		// Ctrl+C (0x03) or else can't break the program except with Ctrl+Break !
		if buf[0] == 0x03 {
			fmt.Println("\nCtrl+C detected → breaking gracefully")
			_ = term.Restore(fd, oldState)
			cleanExitFn()
		}

		// Alt+X / Alt+R → ESC + key
		if buf[0] == 0x1b && n >= 2 {
			switch buf[1] {
			case 'x', 'X':
				fmt.Println("\nAlt+X detected → clean exit")
				_ = term.Restore(fd, oldState)
				cleanExitFn()
			case 'r', 'R':
				fmt.Println("\nAlt+R detected → reloading config")
				//_ = term.Restore(fd, oldState)
				reloadFn()
			}
		}

		_, err = term.MakeRaw(fd)
		if err != nil {
			fmt.Println("\nFailed to makeraw the terminal...")
			return
		}
	}
}
