// eh, there's no problem there with &flags being on stack when passed to syscall.WSARecvFrom
package main

import (
	"fmt"
	"net"
	//"runtime"
	"time"
)

func main() {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	port := conn.LocalAddr().(*net.UDPAddr).Port
	fmt.Printf("Listening on 127.0.0.1:%d\n", port)

	// Sender goroutine — hammers packets continuously
	go func() {
		sender, err := net.Dial("udp4", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			panic(err)
		}
		defer sender.Close()
		payload := []byte("x")
		for {
			time.Sleep(20 * time.Millisecond)
			sender.Write(payload)
			// No sleep — maximum rate to maximize race window hits
		}
	}()

	buf := make([]byte, 2)
	start := time.Now()
	count := 0

	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			fmt.Printf("ReadFromUDP error after %d reads: %v\n", count, err)
			return
		}
		count++
		_ = n
		if count%100000 == 0 {
			fmt.Printf("[%v] %d reads so far...\n", time.Since(start).Round(time.Second), count)
		}
		// // Aggressively reclaim old stack pages and reuse them,
		// // turning silent corruption into an actual crash
		// if count%100 == 0 {
		// 	// Allocate stuff that might land on the reclaimed old stack page,
		// 	// so when Windows writes there it corrupts a live object
		// 	pressure := make([]*[1024]byte, 500)
		// 	for i := range pressure {
		// 		pressure[i] = new([1024]byte)
		// 	}
		// 	runtime.GC()
		// 	_ = pressure
		// }
	}
}
