// now checking if &flags being on stack is bad, so far doesn't seem to be!
//
// old: https://github.com/golang/go/issues/77975
package main

import (
	"fmt"
	"golang.org/x/sys/windows"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	modkernel32                   = windows.NewLazySystemDLL("kernel32.dll")
	procCreateIoCompletionPort    = modkernel32.NewProc("CreateIoCompletionPort")
	procGetQueuedCompletionStatus = modkernel32.NewProc("GetQueuedCompletionStatus")
)

var (
	modws2_32      = windows.NewLazySystemDLL("ws2_32.dll")
	procWSASocketW = modws2_32.NewProc("WSASocketW")
)

const WSA_FLAG_OVERLAPPED = 0x01

func wsaSocket() syscall.Handle {
	// WSASocketW(af, type, protocol, lpProtocolInfo, g, dwFlags)
	h, _, err := procWSASocketW.Call(
		uintptr(syscall.AF_INET),
		uintptr(syscall.SOCK_DGRAM),
		uintptr(syscall.IPPROTO_UDP),
		0, // no protocol info
		0, // no socket group
		uintptr(WSA_FLAG_OVERLAPPED),
	)
	if syscall.Handle(h) == syscall.InvalidHandle {
		panic(err)
	}
	return syscall.Handle(h)
}

func createIOCP() syscall.Handle {
	h, _, err := procCreateIoCompletionPort.Call(
		uintptr(syscall.InvalidHandle),
		0, 0, 1,
	)
	if h == 0 {
		panic(err)
	}
	return syscall.Handle(h)
}

func bindSocketToIOCP(iocp syscall.Handle, sock syscall.Handle) {
	h, _, err := procCreateIoCompletionPort.Call(
		uintptr(sock),
		uintptr(iocp),
		0, // completion key
		0,
	)
	if h == 0 {
		panic(err)
	}
}

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

// func runTest() {
// conn, _ := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
// h := getHandle(conn)
// defer conn.Close()

// done := make(chan struct{})

// go func() {
// // 1. Setup variables on a fresh stack
// var flags uint32
// var qty uint32
// buf := make([]byte, 512)
// //o := syscall.Overlapped{} // because this is on stack(like in Go 1.26.0) it crashes! https://github.com/golang/go/issues/77975
// o := new(syscall.Overlapped) // now it's on heap so should be good!
// wsaBuf := syscall.WSABuf{Len: uint32(len(buf)), Buf: &buf[0]}

// // 2. Start Syscall
// //syscall.WSARecvFrom(h, &wsaBuf, 1, &qty, &flags, nil, nil, &o, nil)
// syscall.WSARecvFrom(h, &wsaBuf, 1, &qty, &flags, nil, nil, o, nil)

// // 3. MOVE THE STACK NOW
// // We grow the stack so 'flags' is moved to a new physical address.
// growAndCorrupt(1, 100)

// // 4. TRIGGER WINDOWS WRITE
// // While we are deep in recursion, we send the packet.
// // Windows writes to the OLD physical address of '&flags'.
// sendPacket(conn.LocalAddr().String())

// close(done)
// }()

// <-done
// }

// func runTest() {
// // 1. Create a "Rogue" Socket completely invisible to Go's netpoll
// fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
// if err != nil {
// panic(err)
// }
// defer syscall.Closesocket(fd)

// // Bind to 127.0.0.1:0
// addr := &syscall.SockaddrInet4{Port: 0, Addr: [4]byte{127, 0, 0, 1}}
// if err := syscall.Bind(fd, addr); err != nil {
// panic(err)
// }

// // Figure out what port Windows assigned us so we can send to it
// sa, _ := syscall.Getsockname(fd)
// boundPort := sa.(*syscall.SockaddrInet4).Port

// var wg sync.WaitGroup
// wg.Add(1)

// go func() {
// defer wg.Done()

// // 2. Setup variables on the CURRENT stack
// //var flags uint32
// var flags uint32 = 0xAAAAAAAA
// addrBefore := uintptr(unsafe.Pointer(&flags))
// //fmt.Printf("[!] Flags starts at: 0x%x\n", addrBefore)
// var qty uint32
// buf := make([]byte, 2)
// o := new(syscall.Overlapped) // On heap to prevent IOCP/netpoll crashes if they somehow hit

// wsaBuf := syscall.WSABuf{Len: uint32(len(buf)), Buf: &buf[0]}

// // 3. Start Syscall
// errno:=syscall.WSARecvFrom(fd, &wsaBuf, 1, &qty, &flags, nil, nil, o, nil)
// fmt.Printf("WSARecvFrom errno: %v\n", errno)

// // 4. MOVE THE STACK NOW
// growAndCorrupt(1, 100)

// // Record the address AFTER the move
// addrAfter := uintptr(unsafe.Pointer(&flags))

// // 5. TRIGGER WINDOWS WRITE
// sendPacket(fmt.Sprintf("127.0.0.1:%d", boundPort))

// // After growAndCorrupt:
// if flags == 0xAAAAAAAA {
// // If it's still 0xAAAAAAAA, Windows hasn't written to the NEW address yet.
// // But did it write to the OLD address? We can't tell without a crash.
// fmt.Println("no write")
// }
// if addrBefore != addrAfter {
// fmt.Printf("[!] Flags was/is at: 0x%x vs 0x%x\n", addrBefore,addrAfter)
// // // DANGEROUS: Peek at the old address (might crash if OS reclaimed the page)
// // // We use a small hack to see if the old memory changed
// // peekOldMemory(addrBefore)
// }
// }()

// wg.Wait()
// }

// func runTest() {
// // Use Go's net package — this socket IS bound to Go's IOCP port
// conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
// if err != nil {
// panic(err)
// }
// defer conn.Close()

// h := getHandle(conn)
// boundPort := conn.LocalAddr().(*net.UDPAddr).Port

// var wg sync.WaitGroup
// wg.Add(1)

// go func() {
// defer wg.Done()

// // flags lives on THIS goroutine's stack
// var flags uint32 = 0xAAAAAAAA
// addrBefore := uintptr(unsafe.Pointer(&flags))

// var qty uint32
// buf := make([]byte, 512)

// // IMPORTANT: Overlapped on heap is fine for heap corruption detection,
// // but flags on stack is what we're hunting
// o := new(syscall.Overlapped)
// wsaBuf := syscall.WSABuf{Len: uint32(len(buf)), Buf: &buf[0]}

// errno := syscall.WSARecvFrom(h, &wsaBuf, 1, &qty, &flags, nil, nil, o, nil)

// // On a Go-netpoller socket this SHOULD return ERROR_IO_PENDING
// if errno != nil && errno != syscall.ERROR_IO_PENDING {
// fmt.Printf("WSARecvFrom failed unexpectedly: %v\n", errno)
// return
// }
// if errno == nil {
// fmt.Println("Completed synchronously — no race window, skipping")
// return
// }

// // errno == ERROR_IO_PENDING: Windows is holding &flags!
// fmt.Printf("IO_PENDING confirmed. flags at: 0x%x\n", addrBefore)

// // NOW force the stack move while Windows holds the old &flags
// growAndCorrupt(1, 100)

// addrAfter := uintptr(unsafe.Pointer(&flags))
// if addrBefore != addrAfter {
// fmt.Printf("[STACK MOVED] 0x%x -> 0x%x\n", addrBefore, addrAfter)
// } else {
// fmt.Println("[no stack move detected]")
// }

// // Trigger completion — Windows will write to addrBefore
// sendPacket(fmt.Sprintf("127.0.0.1:%d", boundPort))

// // Give Windows time to write
// time.Sleep(200 * time.Millisecond)

// // Now compare: what's at the NEW address vs what was at the OLD address
// flagsNow := atomic.LoadUint32(&flags)
// if addrBefore != addrAfter {
// // Peek old memory — dangerous but that's the point
// oldVal := *(*uint32)(unsafe.Pointer(addrBefore))
// fmt.Printf("flags at new addr: 0x%x | old addr value: 0x%x\n", flagsNow, oldVal)
// if flagsNow == 0xAAAAAAAA && oldVal != 0xAAAAAAAA {
// fmt.Println("[BUG CONFIRMED] Windows wrote to old stack address!")
// }
// }
// }()

// wg.Wait()
// }

func runTest() {
	iocp := createIOCP()
	defer syscall.CloseHandle(iocp)

	fd := wsaSocket() //syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
	// if err != nil {
	// panic(err)
	// }
	defer syscall.Closesocket(fd)

	if err := syscall.Bind(fd, &syscall.SockaddrInet4{Port: 0, Addr: [4]byte{127, 0, 0, 1}}); err != nil {
		panic(err)
	}

	// Bind socket to OUR iocp — now WSARecvFrom will truly go async
	bindSocketToIOCP(iocp, fd)

	sa, _ := syscall.Getsockname(fd)
	boundPort := sa.(*syscall.SockaddrInet4).Port

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		var flags uint32 = 0xAAAAAAAA
		addrBefore := uintptr(unsafe.Pointer(&flags))
		var qty uint32
		buf := make([]byte, 512)
		o := new(syscall.Overlapped)
		wsaBuf := syscall.WSABuf{Len: uint32(len(buf)), Buf: &buf[0]}

		errno := syscall.WSARecvFrom(fd, &wsaBuf, 1, &qty, &flags, nil, nil, o, nil)
		if errno != nil && errno != syscall.ERROR_IO_PENDING {
			fmt.Printf("WSARecvFrom failed: %v\n", errno)
			return
		}
		if errno == nil {
			fmt.Println("Completed synchronously — no race window")
			return
		}

		fmt.Printf("IO_PENDING confirmed. &flags = 0x%x\n", addrBefore)

		// Stack move while Windows holds &flags
		growAndCorrupt(1, 100)

		addrAfter := uintptr(unsafe.Pointer(&flags))

		// Trigger completion
		sendPacket(fmt.Sprintf("127.0.0.1:%d", boundPort))
		time.Sleep(200 * time.Millisecond)

		flagsNow := atomic.LoadUint32(&flags)
		fmt.Printf("&flags before: 0x%x  after: 0x%x  moved: %v\n",
			addrBefore, addrAfter, addrBefore != addrAfter)
		fmt.Printf("flags value now: 0x%x\n", flagsNow)

		if addrBefore != addrAfter {
			oldVal := *(*uint32)(unsafe.Pointer(addrBefore))
			fmt.Printf("value at OLD address: 0x%x\n", oldVal)
			if flagsNow == 0xAAAAAAAA && oldVal != 0xAAAAAAAA {
				fmt.Println("[BUG CONFIRMED] Windows wrote to the old stack address!")
			}
		}
	}()

	wg.Wait()
}

func peekOldMemory(addr uintptr) {
	// This is unsafe, but we are ghost hunting.
	// We are checking if the 4 bytes at the OLD address are no longer 0xAAAAAAAA
	val := *(*uint32)(unsafe.Pointer(addr))
	if val != 0xAAAAAAAA {
		fmt.Printf("[SUCCESS] THE GHOST DIED! Old address 0x%x was overwritten with: 0x%x\n", addr, val)
		fmt.Println("This proves Windows wrote to the OLD stack location!")
	} else {
		fmt.Println("[FAIL] Old memory still has 0xAAAAAAAA. Windows hasn't written yet or write failed.")
	}
}

func growAndCorrupt(depth, max int) {
	if depth < max {
		growAndCorrupt(depth+1, max)
		return
	}

	// Now we are on the NEW stack.
	// We need to 'Poison' the memory where the old stack used to be.
	for i := 0; i < 20; i++ {
		// This creates massive garbage to force the GC to recycle
		// the exact memory pages Windows is looking at.
		_ = make([]*int, 1024*1024)
		runtime.GC()

		// Fill the current stack with values that look like pointers
		// but aren't valid (e.g., 0x232)
		poisonStack(100)
	}
}

func poisonStack(n int) {
	if n == 0 {
		return
	}
	var arr [64]uintptr
	for i := range arr {
		// Use a value that will cause a segment fault if dereferenced
		arr[i] = 0xDEADBEEF
	}
	poisonStack(n - 1)
	_ = arr
}

// // This function moves the stack and then sits in a loop creating
// // "pointer heavy" traffic on the stack.
// func growAndCorrupt(depth, max int) {
// if depth < max {
// growAndCorrupt(depth+1, max)
// return
// }

// // Now we are on the NEW stack.
// // The OLD stack memory is now "free" in the eyes of the Go runtime.
// // We want the GC to start using that old memory for something else.
// for i := 0; i < 10; i++ {
// runtime.GC() // Encourage the GC to reclaim the old stack space
// createPointerPressure()
// }
// }

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
	if depth <= 0 {
		return
	}
	// These pointers will likely land on the "Deallocated" old stack area
	var a, b, c *int = new(int), new(int), new(int)
	useOldStackMemory(depth - 1)
	_ = a
	_ = b
	_ = c
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
	time.Sleep(102 * time.Millisecond)

	c, err := net.Dial("udp4", addr)
	if err != nil {
		fmt.Println("Dial error:", err)
		return
	}
	//fmt.Println("Sending trigger packet...")
	c.Write([]byte("crash_me"))
	c.Close()
}
