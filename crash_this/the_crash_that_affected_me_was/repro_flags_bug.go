// https://github.com/golang/go/issues/77975
// fixed in Go commit 1a44be4cecdc742ac6cce9825f9ffc19857c99f3 which is in Go v1.26.2
//addon fix in Go commit 6ab37c1ca59664375786fb2f3c122eb3db98e433 which isn't yet released (i used it tho in my Go 1.27 devel master branch wtw)
package main

import (
	"fmt"
	"net"
	"runtime"
	//"sync"
	"syscall"
	"time"
	//"unsafe"
)

// We need the raw handle from the net.UDPConn
func getHandle(conn *net.UDPConn) syscall.Handle {
	raw, err := conn.SyscallConn()
	if err != nil {
		panic(err)
	}
	var h syscall.Handle
	raw.Control(func(fd uintptr) {
		h = syscall.Handle(fd)
	})
	return h
}

func main() {
	// Set GOMAXPROCS high to increase preemption/collision chances
	runtime.GOMAXPROCS(runtime.NumCPU())

	fmt.Println("Starting reproduction attempt...")
	fmt.Println("If the bug hits, the program will likely crash with 'ret' or 'unwinder' errors.")

	for i := 0; ; i++ {
		if i%100 == 0 {
			fmt.Printf("Iteration %d...\n", i)
		}
		runTest()
	}
}

// func runTest() {
	// // 1. Create a UDP listener on a random port
	// laddr, _ := net.ResolveUDPAddr("udp4", "127.0.0.1:0")
	// conn, err := net.ListenUDP("udp4", laddr)
	// if err != nil {
		// panic(err)
	// }
	// defer conn.Close()
	// h := getHandle(conn)

	// var wg sync.WaitGroup
	// wg.Add(1)

	// // This is our VICTIM goroutine
	// go func() {
		// defer wg.Done()

		// // 2. These variables live on the CURRENT stack
		// var flags uint32
		// var qty uint32
		// buf := make([]byte, 512)
		// o := syscall.Overlapped{}
		
		// // 3. Start the Overlapped I/O
		// // We pass the physical address of 'flags' to the Windows Kernel
		// wsaBuf := syscall.WSABuf{Len: uint32(len(buf)), Buf: &buf[0]}
		// err := syscall.WSARecvFrom(h, &wsaBuf, 1, &qty, &flags, nil, nil, &o, nil)

		// if err != nil && err != syscall.ERROR_IO_PENDING { //WSA_IO_PENDING {
			// return
		// }

		// // --- THE RACE WINDOW OPENS ---
		
		// // 4. FORCE A STACK MOVE
		// // We call a recursive function that demands more stack space than currently allocated.
		// // Go will allocate a new stack and MOVE the current frame (including 'flags') to it.
		// forceStackMove(1, 200)
    // // Call something that uses lots of pointers right where the old stack was
    // useOldStackMemory(50)

		// // 5. Yield to let the GC or other churn happen
		// runtime.Gosched()

		// // --- THE RACE WINDOW CLOSES ---
		
		// // By the time we reach here, if the packet arrives, Windows writes to the OLD address.
	// }()

	// // 6. Trigger the completion
	// // Give the victim a tiny head start to reach the syscall, then send the packet
	// time.Sleep(1 * time.Millisecond)
	// sendPacket(conn.LocalAddr().String())

	// wg.Wait()
// }

func runTest() {
    conn, _ := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
    h := getHandle(conn)
    defer conn.Close()

    done := make(chan struct{})

    go func() {
        // 1. Setup variables on a fresh stack
        var flags uint32
        var qty uint32
        buf := make([]byte, 512)
        o := syscall.Overlapped{} // because this is on stack(like in Go 1.26.0) it crashes! https://github.com/golang/go/issues/77975
        wsaBuf := syscall.WSABuf{Len: uint32(len(buf)), Buf: &buf[0]}

        // 2. Start Syscall
        syscall.WSARecvFrom(h, &wsaBuf, 1, &qty, &flags, nil, nil, &o, nil)

        // 3. MOVE THE STACK NOW
        // We grow the stack so 'flags' is moved to a new physical address.
        growAndCorrupt(1, 100)

        // 4. TRIGGER WINDOWS WRITE
        // While we are deep in recursion, we send the packet.
        // Windows writes to the OLD physical address of '&flags'.
        sendPacket(conn.LocalAddr().String())

        close(done)
    }()

    <-done
}

// This function moves the stack and then sits in a loop creating 
// "pointer heavy" traffic on the stack.
func growAndCorrupt(depth, max int) {
    if depth < max {
        growAndCorrupt(depth+1, max)
        return
    }

    // Now we are on the NEW stack.
    // The OLD stack memory is now "free" in the eyes of the Go runtime.
    // We want the GC to start using that old memory for something else.
    for i := 0; i < 10; i++ {
        runtime.GC() // Encourage the GC to reclaim the old stack space
        createPointerPressure()
    }
}

func createPointerPressure() {
    // We call a function that puts lots of pointers on the stack.
    // One of these might land exactly where the old 'flags' was.
    var pointers [128]*int
    for i := range pointers {
        obj := i
        pointers[i] = &obj
    }
    // Spend a tiny bit of time here so the packet has a chance to land
    time.Sleep(10 * time.Microsecond)
    _ = pointers
}

func useOldStackMemory(depth int) {
    if depth <= 0 { return }
    // These pointers will likely land on the "Deallocated" old stack area
    var a, b, c *int = new(int), new(int), new(int)
    useOldStackMemory(depth - 1)
    _ = a; _ = b; _ = c
}

// forceStackMove uses recursion to force Go to grow the stack.
// Because it's recursive, the current frame must be copied to the new, larger stack.
func forceStackMove(depth, max int) {
	if depth > max {
		return
	}
	var dummy [1024]byte // Use some space to encourage growth
	for i := range dummy {
		dummy[i] = byte(depth)
	}
	forceStackMove(depth+1, max)
	_ = dummy[0] 
}

func sendPacket(addr string) {
	// Small delay to ensure the victim is actually 'Pending'
    time.Sleep(10 * time.Millisecond) 
    
    c, err := net.Dial("udp4", addr)
    if err != nil {
        fmt.Println("Dial error:", err)
        return
    }
    //fmt.Println("Sending trigger packet...")
    c.Write([]byte("crash_me"))
    c.Close()
}