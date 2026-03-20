package main

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ── Constants & Types ───────────────────────────────────────────────────────

const (
	PRJ_VIRTUALIZATION_INSTANCE_HANDLE uintptr = 0 // placeholder
	// Add real HRESULT constants from projectedfslib.h
	S_OK           = 0
	ERROR_ACCESS_DENIED int32 = 5
	ERROR_READ_ONLY     int32 = 19
	// ... more as needed
)

// PRJ_CALLBACK_DATA simplified (real one has more fields!)
type PrjCallbackData struct {
	ProcessId      uint32
	// ... many more fields
}

type PrjCallbacks struct {
	// Pointers to Go callback functions (created with syscall.NewCallback)
	StartDirectoryEnumeration   uintptr
	GetDirectoryEnumeration     uintptr
	EndDirectoryEnumeration     uintptr
	GetFileData                 uintptr
	// ... you need ~10-15 more for full support
	// See: https://learn.microsoft.com/en-us/windows/win32/api/projectedfslib/ns-projectedfslib-prj_callbacks
}

// ── Globals & State ─────────────────────────────────────────────────────────

var (
	projfsLib = windows.NewLazySystemDLL("ProjectedFSLib.dll")

	prjStartVirtualizing = projfsLib.NewProc("PrjStartVirtualizing")
	prjStopVirtualizing  = projfsLib.NewProc("PrjStopVirtualizing")
	// Add more Prj* functions as you implement them

	mu          sync.Mutex
	authHandles = make(map[uintptr]bool) // handle -> authorized

	realKeyPath    = filepath.Join(os.Getenv("APPDATA"), "mysecure", "encrypted_id_rsa.bin")
	encryptionKey  = deriveKey("your-hardcoded-or-securely-stored-master-pass") // CHANGE THIS
	fileContent    []byte // cached decrypted content after first auth
)

func deriveKey(password string) []byte {
	h := sha256.Sum256([]byte(password))
	return h[:]
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func getProcessName(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "unknown"
	}
	defer windows.CloseHandle(h)

	var buf [1024]uint16
	size := uint32(len(buf))
	err = windows.QueryFullProcessImageName(h, 0, &buf[0], &size)
	if err != nil {
		return "unknown"
	}
	path := syscall.UTF16ToString(buf[:size])
	return filepath.Base(path)
}

func decryptKey() ([]byte, error) {
	data, err := os.ReadFile(realKeyPath)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// ── Callbacks (minimal) ─────────────────────────────────────────────────────

func getFileDataCallback(callbackData *PrjCallbackData, data *byte) uintptr { // returns HRESULT
	mu.Lock()
	defer mu.Unlock()

	pid := callbackData.ProcessId
	proc := getProcessName(pid)

	log.Printf("GetFileData from PID %d (%s)", pid, proc)

	allowed := proc == "ssh.exe" || proc == "git.exe"
	if !allowed {
		log.Printf("Denied: not ssh/git")
		return uintptr(ERROR_ACCESS_DENIED)
	}

	// Simple password prompt (console) - replace with GUI later
	if !authHandles[uintptr(unsafe.Pointer(callbackData))] { // per-handle for now
		fmt.Print("Enter password for SSH key: ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		pass := strings.TrimSpace(scanner.Text())

		if sha256.Sum256([]byte(pass)) != sha256.Sum256([]byte("test123")) { // CHANGE THIS
			log.Println("Wrong password")
			return uintptr(ERROR_ACCESS_DENIED)
		}

		// Decrypt once
		var err error
		fileContent, err = decryptKey()
		if err != nil {
			log.Printf("Decrypt failed: %v", err)
			return uintptr(ERROR_ACCESS_DENIED)
		}

		authHandles[uintptr(unsafe.Pointer(callbackData))] = true
		log.Println("Authorized")
	}

	// Provide data (simplified - real needs to handle offset/length)
	// Here we assume full file read
	copy(unsafe.Slice(data, len(fileContent)), fileContent)

	return S_OK
}

// Dummy callbacks (must be defined, even if returning errors)
func dummyCallback(...interface{}) uintptr {
	return uintptr(ERROR_READ_ONLY)
}

// ── Main ────────────────────────────────────────────────────────────────────

func main() {
	// 1. Create .ssh if needed
	sshDir := filepath.Join(os.Getenv("USERPROFILE"), ".ssh")
	os.MkdirAll(sshDir, 0700)

	// 2. Prepare callbacks
	callbacks := PrjCallbacks{
		GetFileData: syscall.NewCallback(getFileDataCallback),
		// Fill other fields with syscall.NewCallback(dummyCallback)
		// You MUST implement at least Start/End/GetDirectoryEnumeration for dir listing
	}

	// 3. Start virtualization
	rootPath, _ := syscall.UTF16PtrFromString(sshDir)
	var instanceID windows.GUID // zero for now, or generate

	r1, _, err := prjStartVirtualizing.Call(
		uintptr(unsafe.Pointer(rootPath)),
		uintptr(unsafe.Pointer(&callbacks)),
		0, // options
		uintptr(unsafe.Pointer(&instanceID)),
	)
	if int32(r1) < 0 {
		log.Fatalf("PrjStartVirtualizing failed: %v (HRESULT: 0x%x)", err, r1)
	}

	log.Printf("Virtualization started on %s. Access id_rsa to trigger prompt.", sshDir)
	log.Println("Press Ctrl+C to stop...")

	// Graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	//prjStopVirtualizing.Call(uintptr(instanceID)) // gg, grok
	prjStopVirtualizing.Call(uintptr(unsafe.Pointer(&instanceID)))
	log.Println("Stopped.")
}