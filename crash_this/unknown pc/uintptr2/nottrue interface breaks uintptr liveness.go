package main

import (
"math/rand"
	"net"
	"time"
"encoding/binary"
	"errors"
	"fmt"
	"runtime"
  "math"
	"strings"

	"unsafe"

	"golang.org/x/sys/windows"
)

type BoundProc struct {
	Proc  LazyProcish
	Check WinCheckFunc
}

//XXX: crashes without this, doh:
//go:uintptrescapes
func (b *BoundProc) Call(args ...uintptr) (uintptr, uintptr, error) {
	return WinCall(b.Proc, b.Check, args...)
}

type WinCheckFunc func(r1 uintptr) bool

func NewBoundProc(dll *windows.LazyDLL, name string, check WinCheckFunc) *BoundProc {

	if check == nil {
		panic("NewBoundProc: nil WinCheckFunc passed as arg")
	}

	return &BoundProc{
		Proc:  RealProc2(dll, name),
		Check: check,
	}
}

var (
	// CheckBool identifies a failure for functions returning a Windows BOOL in r1.
	// In the Windows API, a 0 (FALSE) indicates that the function failed.
	CheckBool WinCheckFunc = func(r1 uintptr) bool { return r1 == 0 }

	// CheckHandle identifies a failure for functions returning a HANDLE in r1.
	// Many Windows APIs return INVALID_HANDLE_VALUE (all bits set to 1) on failure.
	// ^uintptr(0) is the Go-idiomatic way to represent -1 as an unsigned pointer.
	CheckHandle WinCheckFunc = func(r1 uintptr) bool { return r1 == ^uintptr(0) }

	// CheckNull identifies a failure for functions returning a pointer or a handle in r1
	// where a NULL value (0) indicates the operation could not be completed.
	CheckNull WinCheckFunc = func(r1 uintptr) bool { return r1 == 0 }

	// CheckHRESULT identifies a failure for functions that return an HRESULT in r1.
	// An HRESULT is a 32-bit value where a negative number (high bit set)
	// indicates an error, while 0 or positive values indicate success.
	CheckHRESULT WinCheckFunc = func(r1 uintptr) bool { return int32(r1) < 0 }

	// CheckErrno identifies a failure for Win32 APIs that return a DWORD error code in r1.
	// In this convention, 0 (ERROR_SUCCESS) means success, any non-zero value is a failure.
	CheckErrno WinCheckFunc = func(r1 uintptr) bool { return r1 != 0 }
)

type LazyProcish interface {
	// Name returns the name of the procedure (used in error messages).
	//Why Name() instead of a field? Because interfaces in Go cannot require fields — only methods
	Name() string

	// Call invokes the Windows procedure with the given arguments.
	// Signature must match windows.LazyProc.Call exactly.
	Call(a ...uintptr) (r1, r2 uintptr, lastErr error)
}

// realLazyProc wraps *windows.LazyProc to satisfy LazyProcish.
//
// Embedding gives us .Call() for free via promotion.
type realLazyProc struct {
	*windows.LazyProc
}

// Name implements LazyProcish.
//
// Returns the procedure name for use in error messages.
func (r *realLazyProc) Name() string {
	return r.LazyProc.Name
}

func RealProc(p *windows.LazyProc) LazyProcish {
	return &realLazyProc{LazyProc: p}
}

func RealProc2(dll *windows.LazyDLL, name string) LazyProcish {
	if dll == nil {
		panic("RealProc2: nil dll")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		panic("RealProc2: empty proc name")
	}
	return RealProc(dll.NewProc(name))
}

func CheckWinResult(
	//can be empty
	operationNameToIncludeInErrorMessages string,
	isFailure WinCheckFunc,
	//onFail func(err error),
	r1 uintptr,
	callErr error,
) error {
	if !isFailure(r1) {
		// Success: return nil so 'if err != nil' behaves normally.
		return nil
	}

	// Normalize callErr: treat ERROR_SUCCESS as nil
	if callErr != nil && errors.Is(callErr, windows.ERROR_SUCCESS) {
		callErr = nil
	}

	// If callErr is missing/useless, try to recover from r1
	if callErr == nil {
		// Many Win32 APIs (e.g. GetExtendedUdpTable) return the error in r1.
		// Only treat r1 as an errno if it's non-zero.
		if r1 != 0 {
			errno := windows.Errno(r1)

			// Defensive: avoid ever wrapping ERROR_SUCCESS
			if !errors.Is(errno, windows.ERROR_SUCCESS) {
				// since r1 != 0 already, this is bound to never be ERROR_SUCCESS here, unless r1 != 0 can ever be ERROR_SUCCESS, unsure.
				return fmt.Errorf("%q windows call failed with error: %w", operationNameToIncludeInErrorMessages, errno)
			}
		}

		// Fallback: truly unknown failure
		return fmt.Errorf(
			"%q windows call reported failure (ret=%d) but no usable error was provided",
			operationNameToIncludeInErrorMessages,
			r1,
		)
	}

	// Normal path: we have a meaningful callErr
	return fmt.Errorf("%q windows call failed with error: %w", operationNameToIncludeInErrorMessages, callErr)

}

//XXX: hmm, doesn't crash without this, I guess this means, yes, it's transitive!
//go:uintptrescapes
func WinCall(proc LazyProcish, check WinCheckFunc, args ...uintptr) (uintptr, uintptr, error) {
	if proc == nil {
		panic(fmt.Errorf("WinCall: nil proc"))
	}

	op := strings.TrimSpace(proc.Name())
	if op == "" {
		op = "UnspecifiedWinApi"
	}

	// churn memory before the call
	churn()
	stackChurn(64) // grow stack
	runtime.GC()   // encourage shrink afterwards
	runtime.Gosched()
	smashStack()

	// args is a []uintptr, but because of //go:uintptrescapes, the caller
	// has already pinned the memory safely before we get here.
	r1, r2, callErr := proc.Call(args...)
	err := CheckWinResult(op, check, r1, callErr)
	return r1, r2, err
}

// ---- Test harness ----

var sink any

func churn() {
	// Force GC + stack pressure
	for i := 0; i < 100; i++ {
		b := make([]byte, 1<<20) // 1MB
		sink = b
	}
	//runtime.GC()
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

func main() {
	//c := &realCaller{}

	for i := 0; i < 1_000_000; i++ {
		//var x uint64 = 0x1122334455667788
		f := frame{
			x: 0xDEADBEEFCAFEBABE, // sentinel
		}

		// pointer -> uintptr
		//p := uintptr(unsafe.Pointer(&f.x)) // XXX: yes this crashes if here (normal, doh!)

	// // churn memory before the call
  // //XXX: if you do these here instread of in WinCall, then it may actually crash
	// churn()
	// stackChurn(64) // grow stack
	// runtime.GC()   // encourage shrink afterwards
	// runtime.Gosched()
	// smashStack()


		// call through interface
		procGetSystemTimeAsFileTime.Call(
			//p, //XXX: crashes
			uintptr(unsafe.Pointer(&f.x)), // XXX: doesn't crash!
		)
    
    //var clientAddr *net.TCPAddr
    rand.Seed(time.Now().UnixNano())
	port := rand.Intn(65535-1024+1) + 1024
	clientAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: port}
    
    pid, exe, err := PidAndExeForTCP(clientAddr)
    _=pid
    _=exe
    _=err
    clientAddr2 := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port}
    pid, exe, err = PidAndExeForUDP(clientAddr2)
    _=pid
    _=exe
    _=err


		if f.x == 0xDEADBEEFCAFEBABE {
			panic("write did NOT land in f.x (stale pointer)")
		}

// Use current process PID — always exists, always succeeds
    //pid = 1316//windows.GetCurrentProcessId()
    pids := []uint32{1316, 1300, windows.GetCurrentProcessId(), } // replace with real PIDs
    for _, pid := range pids {
    name, err := ExePathFromPID(pid)
    _ = name
    _ = err
    exe, err2 := GetProcessName(pid)
    _ = exe
    _ = err2
    }
		if i%100 == 0 {
			fmt.Println("ok", i)
		}
	}
}

var procGetSystemTimeAsFileTime = NewBoundProc(Kernel32, "GetSystemTimeAsFileTime", CheckErrno)
//var kernel32 = windows.NewLazySystemDLL("kernel32.dll")

func smashStack() {
	var big [65536]byte
	for i := range big {
		big[i] = 0xCC
	}
}

type frame struct {
	pre  uint64
	x    uint64 // technically should be: struct { LowDateTime, HighDateTime uint32 } but we don't care about using it
	post uint64
}


// GetExtendedTCPTable retrieves the system TCP table.
// It follows the same contract as GetExtendedUDPTable.
func GetExtendedTCPTable(order bool, family uint32) ([]byte, error) {
	return callWithRetry("GetExtendedTCPTable", 0, func(bufPtr *byte, s *uint32) error {
		_, _, err := procGetExtendedTcpTable.Call(
			uintptr(unsafe.Pointer(bufPtr)),
			uintptr(unsafe.Pointer(s)),
			boolToUintptr(order),
			uintptr(family),
			uintptr(TCP_TABLE_OWNER_PID_ALL), // Value 5: Get all states + PID
			0,
		)
		//these keepalives are probably not needed but hey, ChatGPT.
		runtime.KeepAlive(bufPtr)
		runtime.KeepAlive(s)
		return err
	})
}

func GetExtendedUDPTable(order bool, family uint32) ([]byte, error) {
	return callWithRetry("GetExtendedUDPTable", 0, func(bufPtr *byte, s *uint32) error {
		_, _, err := procGetExtendedUdpTable.Call(
			uintptr(unsafe.Pointer(bufPtr)),
			uintptr(unsafe.Pointer(s)),
			boolToUintptr(order),
			uintptr(family),
			uintptr(UDP_TABLE_OWNER_PID),
			0,
		)
		//these keepalives are probably not needed but hey, ChatGPT.
		runtime.KeepAlive(bufPtr)
		runtime.KeepAlive(s)
		return err
	})
}

func callWithRetry(who string, initialSize uint32, call func(bufPtr *byte, s *uint32) error) ([]byte, error) {
	size := initialSize
	const MAX_RETRIES = 10
	for tries := 1; tries <= MAX_RETRIES; tries++ { // tries will be 1, 2, 3, ..., MAX_RETRIES
		//for tries := 0; tries < MAX_RETRIES; tries++ { // tries will be 0, 1, 2, ..., MAX_RETRIES-1
		//for tries := range MAX_RETRIES { // tries will be 0, 1, 2, ..., MAX_RETRIES-1
//		fmt.Printf("!%s before6 try %d, initialSize=%d size=%d\n", who, tries, initialSize, size)
		// If size is 0, we're just probing. If > 0, we're allocating.
		var buf []byte
		//var p uintptr
		var ptr *byte = nil //implied anyway
		// const canary uint64 = 0xDEADBEEFCAFEBABE
		// var canaryOffset int
		if size > 0 {
			buf = make([]byte, size) //+8) // 8 extra bytes
			//canaryOffset = len(buf) - 8
			// write canary at the end
			//binary.LittleEndian.PutUint64(buf[canaryOffset:], canary)
			ptr = &buf[0] // Keep it as a real, GC-visible pointer
			/*
				fmt.Printf with the %p verb treats a slice value specially: for a slice,
					%p prints the address of the first element (the Data pointer), not the address of the slice descriptor.
					The slice variable itself is a three-word header (pointer, len, cap) stored on the stack (or heap).
					The header's address is &buf; the header's Data field (pointer to element 0) is what fmt prints for %p when given a slice.

				So:

				    buf (the slice) ≠ &buf (address of the header).
				    fmt.Printf("%p", buf) prints buf's Data pointer (same as &buf[0] when len>0).
				    To print the header address use fmt.Printf("%p", &buf). To print the Data pointer explicitly
					use fmt.Printf("%p", unsafe.Pointer(&buf[0])) (only when len>0).

			*/
//			fmt.Printf("!%s middle7(created buf) try %d, buf=%p ptr=%p size=%d len(buf)=%d\n", who, tries, buf, ptr, size, len(buf))
		}
//		fmt.Printf("!%s before7 try %d, ptr=%p &size=%p size=%d\n", who, tries, ptr, &size, size)
		err := call(ptr, &size)
		runtime.KeepAlive(buf) // probably not needed but hey, ChatGPT.
//fmt.Printf("!%s after7 try %d, ptr=%p &size=%p size=%d\n", who, tries, ptr, &size, size)
		// //check canary immediately after
		// if buf != nil { // guard for first iteration when size==0
		// 	if binary.LittleEndian.Uint64(buf[canaryOffset:]) != canary {
		// 		panic(fmt.Sprintf("CANARY SMASHED in callWithRetry after call, allocSize=%d, apiReportedSize=%d", canaryOffset, size))
		// 	}
		// }
		if err == nil {
//			fmt.Printf("!%s middle7(ret ok) try %d, buf=%p len(buf)=%d size=%d\n", who, tries, buf, len(buf), size)
			if uint64(size) > uint64(len(buf)) {
				panic("impossible: size is bigger than len(buf)")
			}
			//return buf, nil // epic fail here
			return buf[:size], nil // fixed one issue!
		}

		// Windows uses both INSUFFICIENT_BUFFER and MORE_DATA
		// to signal that we need a bigger boat.
		//GetExtendedUdpTable usually returns ERROR_INSUFFICIENT_BUFFER when the buffer is too small.
		//EnumServicesStatusEx (and many Enumeration APIs) returns ERROR_MORE_DATA.
		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) &&
			!errors.Is(err, windows.ERROR_MORE_DATA) {
			fmt.Printf("!%s middle7_2(ret err) try %d, err=%v\n", who, tries, err)
			return nil, err
		}
		// Loop continues, using the updated 'size' from the failed call
		//however:
		// If size didn't increase but we still got an error,
		// we should nudge it upward to prevent an infinite loop.
		// We use uint64 casts to satisfy gosec G115.
		// 1. Convert both to uint64 to compare safely without narrowing (Fixes G115)
		if uint64(size) <= uint64(len(buf)) {
			fmt.Printf("!%s before8 try %d, size=%d buf=%p len(buf)=%d\n", who, tries, size, buf, len(buf))
			// 2. Check for overflow before adding 1024
			const increment = 1024
			const MaxInt = math.MaxUint32
			if MaxInt-size < increment {
				fmt.Printf("!%s middle8 try %d\n", who, tries)
				return nil, fmt.Errorf("buffer size(%d) would overflow uint32(%d) if adding %d", size, MaxInt, increment)
			}
			size += increment
			fmt.Printf("!%s after8 try %d, new size=%d\n", who, tries, size)
		}
		//fmt.Printf("!%s after6(end of for) try %d\n", who, tries)
	}
	return nil, fmt.Errorf("buffer growth exceeded max retries(%d)", MAX_RETRIES)
}

// boolToUintptr converts a Go bool to a uintptr (1 for true, 0 for false)
// for use in Windows syscalls.
//
// boolToUintptr performs an explicit conversion from a Go bool to a
// Windows-compatible BOOL (uintptr(1) for true, uintptr(0) for false).
// This is required because Go bools cannot be directly cast to numeric types.
//
//go:inline
func boolToUintptr(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

var (
	Iphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")
	//procGetExtendedUdpTable = Iphlpapi.NewProc("GetExtendedUdpTable")
	procGetExtendedUdpTable = NewBoundProc(Iphlpapi, "GetExtendedUdpTable", CheckErrno)
	procGetExtendedTcpTable = NewBoundProc(Iphlpapi, "GetExtendedTcpTable", CheckErrno)
  )
  
  const (
  	UDP_TABLE_OWNER_PID     = 1 // MIB_UDPTABLE_OWNER_PID
	TCP_TABLE_OWNER_PID_ALL = 5
  )
  
  func PidAndExeForUDP(clientAddr *net.UDPAddr) (uint32, string, error) {
	//capital P in PidAndExeForUDP means exported, apparently!
	if clientAddr == nil {
		return 0, "", errors.New("nil clientAddr")
	}
	ip4 := clientAddr.IP.To4()
	if ip4 == nil {
		return 0, "", errors.New("only IPv4 supported")
	}
	port := uint16(clientAddr.Port)

	buf, err := GetExtendedUDPTable(false, AF_INET)
	if err != nil {
		return 0, "", err
	}

	if buf == nil {
		return 0, "", errors.New("GetExtendedUdpTable returned empty buffer which means there were no UDP entries in the table")
	}

	// Buffer layout: DWORD dwNumEntries; then array of MIB_UDPROW_OWNER_PID entries.
	if len(buf) < 4 {
		return 0, "", errors.New("GetExtendedUdpTable returned too small buffer")
	}
	num := binary.LittleEndian.Uint32(buf[:4])
	const rowSize = 12 // MIB_UDPROW_OWNER_PID has 3 DWORDs = 12 bytes
	offset := 4
	//var owningPid uint32
	for i := uint32(0); i < num; i++ {
		if offset+rowSize > len(buf) {
			panic(fmt.Sprintf("attempted to read beyond buffer in buf=%p len(buf)=%d offset=%d rowSize=%d i=%d\n", buf, len(buf), offset, rowSize, i))
			//break
		}
		localAddr := binary.LittleEndian.Uint32(buf[offset : offset+4])
		localPortRaw := binary.LittleEndian.Uint32(buf[offset+4 : offset+8])

		// localPortRaw stores port in network byte order in low 16 bits.
		localPort := uint16(localPortRaw & 0xFFFF)
		localPort = (localPort>>8)&0xFF | (localPort&0xFF)<<8 // convert to host order

		// convert DWORD IP (little-endian) to net.IP
		ipb := []byte{
			byte(localAddr & 0xFF),
			byte((localAddr >> 8) & 0xFF),
			byte((localAddr >> 16) & 0xFF),
			byte((localAddr >> 24) & 0xFF),
		}
		entryIP := net.IPv4(ipb[0], ipb[1], ipb[2], ipb[3])

		//fmt.Println("Checking:",entryIP,ip4, localPort, port)

		if localPort == port {
			// treat 0.0.0.0 as wildcard match
			if entryIP.Equal(net.IPv4zero) || entryIP.Equal(ip4) {
				// found PID for our IP:port tuple
				owningPid := binary.LittleEndian.Uint32(buf[offset+8 : offset+12])
				exe, err := ExePathFromPID(owningPid)
				if err != nil {
					//fmt.Println(err)
					// got error due to permissions needed for abs. path? this will work but it's just the .exe:
					//exe, err2 := wincoe.GetProcessName(owningPid) // shadowing is only a warning here, major footgun otherwise.

					var err2 error // Declare err2 so we don't have to use :=
					exe, err2 = GetProcessName(owningPid)

					if err2 != nil {
						return 0, "", fmt.Errorf("pid %d not found for %s, errTransient:'%v', err:'%w'", owningPid, clientAddr.String(), err, err2)
					}

					//_ = exe // enable when trying for shadowing
				}
				return owningPid, exe, nil
			}
		}

		//prepare for next entry
		offset += rowSize
	} //for

	return 0, "", fmt.Errorf("no matching UDP socket entry found for %s (ephemeral port reuse or socket already closed by kernel) thus dno who sent it", clientAddr.String())
}

// clientAddr should be the remote TCP address observed on the server side (e.g., 127.0.0.1:49936).
func PidAndExeForTCP(clientAddr *net.TCPAddr) (uint32, string, error) {
	if clientAddr == nil {
		return 0, "", errors.New("nil clientAddr")
	}
	ip4 := clientAddr.IP.To4()
	if ip4 == nil {
		return 0, "", errors.New("only IPv4 supported")
	}
	port := uint16(clientAddr.Port)

	// Fetch the table
	buf, err := GetExtendedTCPTable(false, AF_INET) //FIXME: do I need here to include the AF_INET6 ?! probably, and for UDP func too!
	if err != nil {
		return 0, "", err
	}
	if buf == nil {
		return 0, "", errors.New("GetExtendedTcpTable returned empty buffer")
	}

	if len(buf) < 4 {
		return 0, "", errors.New("GetExtendedTcpTable buffer too small for header")
	}

	num := binary.LittleEndian.Uint32(buf[:4])

	// MIB_TCPROW_OWNER_PID structure:
	// 0: dwState (4 bytes)
	// 4: dwLocalAddr (4 bytes)
	// 8: dwLocalPort (4 bytes)
	// 12: dwRemoteAddr (4 bytes)
	// 16: dwRemotePort (4 bytes)
	// 20: dwOwningPid (4 bytes)
	const rowSize = 24
	offset := 4

	for i := uint32(0); i < num; i++ {
		if offset+rowSize > len(buf) {
			break
		}

		// Extract fields based on the 24-byte MIB_TCPROW_OWNER_PID layout
		localAddrRaw := binary.LittleEndian.Uint32(buf[offset+4 : offset+8])
		localPortRaw := binary.LittleEndian.Uint32(buf[offset+8 : offset+12])
		owningPid := binary.LittleEndian.Uint32(buf[offset+20 : offset+24])

		// Advance offset for next iteration
		offset += rowSize

		// Port conversion (Network Byte Order in low 16 bits)
		localPort := uint16(localPortRaw & 0xFFFF)
		localPort = (localPort>>8)&0xFF | (localPort&0xFF)<<8

		if localPort == port {
			// Convert DWORD IP (little-endian) to net.IP
			entryIP := net.IPv4(
				byte(localAddrRaw&0xFF),
				byte((localAddrRaw>>8)&0xFF),
				byte((localAddrRaw>>16)&0xFF),
				byte((localAddrRaw>>24)&0xFF),
			)

			// Match logic (Wildcard 0.0.0.0 or specific IP)
			if entryIP.Equal(net.IPv4zero) || entryIP.Equal(ip4) {
				exe, err := ExePathFromPID(owningPid)
				if err != nil {
					// Fallback to process name if path is inaccessible
					var err2 error
					exe, err2 = GetProcessName(owningPid)
					if err2 != nil {
						return 0, "", fmt.Errorf("pid %d found but exe lookup failed: %w", owningPid, err2)
					}
				}
				return owningPid, exe, nil
			}
		}
	}

	return 0, "", fmt.Errorf("no TCP owner found for %s", clientAddr.String())
}

func ExePathFromPID(pid uint32) (string, error) {
	return QueryFullProcessName(pid)
}

func QueryFullProcessName(pid uint32) (string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", fmt.Errorf("OpenProcess failedfor PID %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)

	// Start with MAX_PATH (260)
	//Yes, size remains a uint32 on both x86 and x64. This is because the Windows API function QueryFullProcessImageNameW
	// explicitly defines that parameter as a PDWORD (a pointer to a 32-bit unsigned integer), regardless of the processor architecture.
	size := uint32(windows.MAX_PATH)
	//size := uint32(3) // for tests
	var tries uint64 = 1
	for {
		buf := make([]uint16, size)
		currentCap := uint64(len(buf))
		if currentCap != uint64(size) { // must cast else compile error!
			impossibiru(fmt.Sprintf("currentCap(%d) != size(%d), after %d tries", currentCap, size, tries))
		}

		// Note: QueryFullProcessNameW expects 'size' to include the null terminator
		// on input, and returns the length WITHOUT the null terminator on success.
		// _, _, err = callQueryFullProcessName(
		// 	h,
		// 	0,
		// 	&buf[0],
		// 	&size,
		// )
		_, _, err = procQueryFullProcessName.Call(
			uintptr(h),
			0,
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(unsafe.Pointer(&size)),
		)

		if err == nil {
			// Success! Convert the returned size to string
			//UTF16ToString is a function that looks for a 0x0000 (null).
			//size is just a number the API handed back, so let's not trust it, thus use full 'buf'
			return windows.UTF16ToString(buf), nil
		}

		// Check if the error is specifically "Buffer too small"
		// syscall.ERROR_INSUFFICIENT_BUFFER = 0x7A
		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
			return "", fmt.Errorf("QueryFullProcessNameW failed after %d tries, err: '%w'", tries, err)
		}
		//else the desired 'size' now includes the nul terminator, so no need to +1 it

		// currentCap is what we just allocated; nextSize is what the API told us it wants.
		nextSize := uint64(size) //this is api suggested size now! ie. modified! so it's not same as currentCap!

		// If API didn't suggest a larger size, we manually double.
		if nextSize <= currentCap {
			nextSize = currentCap * 2
		}

		if currentCap < MaxExtendedPath && nextSize > MaxExtendedPath {
			// cap it once! in case we doubled it or (unlikely)api suggested more!(in the latter case it will fail the next syscall)
			nextSize = MaxExtendedPath
		}

		// Stern check against the Windows limit (32767) and the uint32 limit.
		if nextSize > MaxExtendedPath || nextSize > math.MaxUint32 {
			return "", fmt.Errorf("buffer size %d exceeds limit, after %d tries", nextSize, tries)
		}

		size = uint32(nextSize)
		tries += 1
	} // infinite 'for'
}

var (
	Kernel32 = windows.NewLazySystemDLL("kernel32.dll")

procQueryFullProcessName = NewBoundProc(Kernel32, "QueryFullProcessImageNameW", CheckBool)
	// procCreateToolhelp32Snapshot = Kernel32.NewProc("CreateToolhelp32Snapshot")
	procCreateToolhelp32Snapshot = NewBoundProc(Kernel32, "CreateToolhelp32Snapshot", CheckHandle)
	// procProcess32First           = Kernel32.NewProc("Process32FirstW")
	procProcess32First = NewBoundProc(Kernel32, "Process32FirstW", CheckBool)
	// procProcess32Next            = Kernel32.NewProc("Process32NextW")
	procProcess32Next = NewBoundProc(Kernel32, "Process32NextW", CheckBool)
  )
  
  const 	AF_INET  = 2

func impossibiru(msg string) {
	panic(fmt.Sprintf("Impossible: '%s'", msg))
}

//MaxExtendedPath is the maximum character count supported by the Unicode (W) versions of Windows API functions when using the \\?\ prefix, and it's the limit for QueryFullProcessNameW.
// don't set a type so it can be compared with other types without error-ing about mismatched types!
const MaxExtendedPath = 32767

// Static assertions to ensure constants are "stern" enough.
// This block will fail to compile if the conditions are not met.
const (
	// Ensure MaxExtendedPath isn't accidentally set higher than what a uint32 can hold.
	_ = uint32(MaxExtendedPath)
)

// Ensure MaxExtendedPath is at least as large as the legacy MAX_PATH (260).
var _ = [MaxExtendedPath - 260]byte{}

func GetProcessName(pid uint32) (string, error) {
	snapshot, err := CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	err = Process32First(snapshot, &entry)
	for err == nil {
		//TODO: make a hard limit here, so it doesn't loop infinitely just in case.
		if entry.ProcessID == pid {
			return windows.UTF16ToString(entry.ExeFile[:]), nil
		}
		err = Process32Next(snapshot, &entry)
	}

	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return "", err
	}
	return "", fmt.Errorf("not found, err: %w", err)
}

const (
	// TH32CS_SNAPHEAPLIST includes all heap lists of the process in the snapshot.
	TH32CS_SNAPHEAPLIST = 0x00000001

	// TH32CS_SNAPPROCESS includes all processes in the system in the snapshot.
	TH32CS_SNAPPROCESS = 0x00000002

	// TH32CS_SNAPTHREAD includes all threads in the system in the snapshot.
	TH32CS_SNAPTHREAD = 0x00000004

	// TH32CS_SNAPMODULE includes all modules of the process in the snapshot.
	//TH32CS_SNAPMODULE enumerates all modules for the process, but on a 64-bit process, it only includes modules of the same bitness as the caller (so a 64-bit process sees 64-bit modules).
	//If you only pass TH32CS_SNAPMODULE in a 64-bit process, you will not see 32-bit modules of a 32-bit process, ergo you need TH32CS_SNAPMODULE32 too.
	TH32CS_SNAPMODULE = 0x00000008

	// TH32CS_SNAPMODULE32 includes 32-bit modules of the process in the snapshot.
	//TH32CS_SNAPMODULE32 explicitly requests 32-bit modules, which is only relevant if your process is 64-bit and you want to see 32-bit modules of a 32-bit process.
	TH32CS_SNAPMODULE32 = 0x00000010

	// TH32CS_SNAPALL is a convenience constant to include all object types.
	TH32CS_SNAPALL = TH32CS_SNAPHEAPLIST | TH32CS_SNAPPROCESS | TH32CS_SNAPTHREAD | TH32CS_SNAPMODULE | TH32CS_SNAPMODULE32

	// TH32CS_INHERIT indicates that the snapshot handle is inheritable.
	TH32CS_INHERIT = 0x80000000
)

func CreateToolhelp32Snapshot(dwFlags, th32ProcessID uint32) (windows.Handle, error) {
	// r1, _, err := callCreateToolhelp32Snapshot(
	// 	dwFlags,
	// 	th32ProcessID,
	// )
	r1, _, err := procCreateToolhelp32Snapshot.Call(
		uintptr(dwFlags),
		uintptr(th32ProcessID),
	)
	if err != nil {
		return 0, err
	}
	return windows.Handle(r1), nil
}

// // CreateProcessSnapshot is a convenience wrapper for creating a snapshot of all processes.
// //
// // Internally calls CreateToolhelp32Snapshot with TH32CS_SNAPPROCESS and PID 0.
// func (l *Exported) CreateProcessSnapshot() (windows.Handle, error) {

// 	return l.CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0)
// }

// Process32First wraps callProcess32First.
func Process32First(snapshot windows.Handle, entry *windows.ProcessEntry32) error {
	_, _, err := procProcess32First.Call(uintptr(snapshot), uintptr(unsafe.Pointer(entry)))
	//_, _, err := callProcess32First(snapshot, entry)
	return err
}

// Process32Next wraps callProcess32Next.
func Process32Next(snapshot windows.Handle, entry *windows.ProcessEntry32) error {
	_, _, err := procProcess32Next.Call(uintptr(snapshot), uintptr(unsafe.Pointer(entry)))
	//_, _, err := callProcess32Next(snapshot, entry)
	return err
}