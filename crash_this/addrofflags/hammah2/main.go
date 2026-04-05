package main

import (
	"fmt"
	"net"
	"runtime"
	"time"
)

func main() {
	//runtime.GOMAXPROCS(runtime.NumCPU()) // maximize parallel GC chance

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	port := conn.LocalAddr().(*net.UDPAddr).Port
	fmt.Printf("Listening on 127.0.0.1:%d\n", port)

	go sender(port)

	// for {
	// // 1. Grow the stack large before each read
	// // This makes the GC want to shrink it afterward
	// growStack(1, 200)

	// // 2. Now do the read — goroutine has a large stack,
	// // GC may shrink it while WSARecvFrom is pending
	// buf := make([]byte, 2) // small buf keeps goroutine stack small initially
	// readOnce(conn, buf)

	// // 3. Aggressively trigger GC to shrink the now-idle large stack
	// runtime.GC()
	// runtime.GC()
	// }

	// Hammer GC from a separate goroutine continuously
	// so it's always running, maximizing chance it hits the window
	go func() {
		for {
			runtime.GC()
			// No sleep — maximum pressure
		}
	}()

	//// Multiple concurrent GC hammers — one per CPU
	// for range runtime.NumCPU() {
	// 	go func() {
	// 		for {
	// 			runtime.GC()
	// 		}
	// 	}()
	// }

	go func() {
		type node struct {
			next *node
			val  uint64
		}

		for {
			go func() {
				//runtime.GC()
				// Allocate right after GC — most likely to get freshly freed pages
				// including old stack pages from shrunk goroutines
				var live []*node
				for i := 0; i < 10000; i++ {
					n := &node{val: 0xDEADBEEFDEADBEEF}
					if i > 0 {
						n.next = live[i-1]
					}
					live = append(live, n)
				}
				// Check chain integrity
				for i := len(live) - 1; i > 0; i-- {
					if live[i].next == nil {
						fmt.Printf("[CORRUPTION] live[%d].next zeroed — Windows wrote there!\n", i)
					}
					if live[i].next != live[i-1] {
						fmt.Printf("[CORRUPTION] live[%d].next is wrong: %p expected %p\n",
							i, live[i].next, live[i-1])
					}
				}
				_ = live
			}() //goroutine
			time.Sleep(1 * time.Millisecond)
		} //for
	}()

	for i := 0; ; i++ {
		if i%1000 == 0 {
			fmt.Printf("iteration %d\n", i)
		}
		triggerRead(conn, i)
		// Small sleep to not spawn goroutines faster than they complete
		//time.Sleep(1 * time.Millisecond)
	}

	// // Multiple concurrent reader goroutines
	// // more goroutines = more chances to hit the window
	// for range runtime.NumCPU() {
	// 	go func() {
	// 		for {
	// 			// Channel to synchronize — we need the read to happen
	// 			// while the stack is in the right state
	// 			done := make(chan error, 1)
	// 			// Fresh stack frame each iteration via closure
	// 			go func() {

	// 				buf := make([]byte, 2)
	// 				// Before the read, allocate a bunch of objects to fill heap
	// 				// After the stack shrinks, GC reclaims old stack page
	// 				// and may hand it to one of these allocations
	// 				// var live []*uint64
	// 				// for i := 0; i < 1000; i++ {
	// 				// 	v := uint64(0xDEADBEEFDEADBEEF)
	// 				// 	live = append(live, &v)
	// 				// }

	// 				// // Allocate pointer-bearing objects
	// 				// // If Windows zeros 4 bytes of a pointer, GC sees misaligned/invalid pointer
	// 				// type node struct {
	// 				// 	next *node
	// 				// 	val  uint64
	// 				// }

	// 				// var live2 []*node
	// 				// for i := 0; i < 1000; i++ {
	// 				// 	n := &node{val: 0xDEADBEEFDEADBEEF}
	// 				// 	if i > 0 {
	// 				// 		n.next = live2[i-1] // chain them so GC must trace all
	// 				// 	}
	// 				// 	live2 = append(live2, n)
	// 				// }

	// 				growStack(1, 200)
	// 				_, _, err := conn.ReadFromUDP(buf)

	// 				// Now old stack page might be backing one of the `live` objects.
	// 				// When Windows writes 0 (flags) to old addr, it silently zeros
	// 				// 4 bytes of one of these uint64s turning 0xDEADBEEFDEADBEEF
	// 				// into 0xDEADBEEF00000000 or 0x00000000DEADBEEF

	// 				// Verify them all — any corruption means Windows wrote there
	// 				// for i, p := range live {
	// 				// 	if *p != 0xDEADBEEFDEADBEEF {
	// 				// 		fmt.Printf("[CORRUPTION] live[%d] = 0x%x — Windows wrote to it!\n", i, *p)
	// 				// 	}
	// 				// }

	// 				// _ = live
	// 				//_ = live2
	// 				done <- err
	// 			}()
	// 			<-done
	// 		}
	// 	}()
	// }
	// // Just wait
	// select {}
}

func triggerRead(conn *net.UDPConn, i int) {
	type node struct {
		next *node
		val  uint64
	}

	// Channel to synchronize — we need the read to happen
	// while the stack is in the right state
	done := make(chan error, 1)
	go func() {

		// Fresh goroutine = fresh small stack every time
		// This is the key — new goroutine, new small stack each iteration

		// 1. Grow it large so GC will want to shrink it
		// 1. Grow THIS goroutine's stack large
		//growStack(1, i % 200)
		growStack(1, 200)

		// 3. Immediately do the read — GC has already decided to shrink
		// but hasn't executed it yet on this goroutine.
		// The shrink executes at the next safe point, which might be
		// right after WSARecvFrom returns IO_PENDING
		buf := make([]byte, 2)
		// 2. GC now — marks this goroutine's stack as shrink candidate
		// because it's large but mostly idle
		//go runtime.GC()

		_, _, err := conn.ReadFromUDP(buf) // blocks but shrink may fire in that window
		if err != nil {
			fmt.Println(err)
		}
		// // Allocate before read — these are live when old stack page gets freed
		// var live []*node
		// for i := 0; i < 10000; i++ {
		// 	n := &node{val: 0xDEADBEEFDEADBEEF}
		// 	if i > 0 {
		// 		n.next = live[i-1]
		// 	}
		// 	live = append(live, n)
		// }
		// // Stack shrink may have happened during IO_PENDING above.
		// // Old stack page is now freed. Hammer allocations to claim it —
		// // one may land exactly at old &flags address.
		// for i := 0; i < 10000; i++ {
		// 	n := &node{val: 0xDEADBEEFDEADBEEF}
		// 	if i > 0 {
		// 		n.next = live[len(live)-1]
		// 	}
		// 	live = append(live, n)
		// }

		// // GC traces the chain — if Windows wrote 0 to old &flags
		// // and a node.next pointer lives there, GC panics
		// //runtime.GC()

		// // If no panic, scan for silent corruption
		// for i, p := range live {
		// 	if p.val != 0xDEADBEEFDEADBEEF {
		// 		fmt.Printf("[CORRUPTION] live[%d].val = 0x%x\n", i, p.val)
		// 	}
		// }

		// _ = live

		done <- err
	}()

	<-done
}

func readOnce(conn *net.UDPConn, buf []byte) {
	// Each call to ReadFromUDP is a fresh goroutine stack frame.
	// After growStack above, this goroutine has a large stack.
	// GC may decide to shrink it while WSARecvFrom is IO_PENDING.
	_, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		fmt.Println("read error:", err)
	}
}

func growStack(depth, max int) {
	if depth >= max {
		// At max depth — now sit here briefly so GC sees a large stack
		// and marks it as a shrink candidate
		runtime.Gosched()
		return
	}
	var pad [512]byte
	pad[0] = byte(depth)
	growStack(depth+1, max)
	_ = pad[0]
}

func sender(port int) {
	c, err := net.Dial("udp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		panic(err)
	}
	defer c.Close()
	payload := []byte("xy") // 1 byte fits in buf of 2
	for {
		c.Write(payload)
		time.Sleep(1 * time.Millisecond)
	}
}
