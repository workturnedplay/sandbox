package main

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

// ── ProjectedFS API loading ─────────────────────────────────────────────────

var (
	projfsLib               = windows.NewLazySystemDLL("ProjectedFSLib.dll")
	prjStartVirtualizing    = projfsLib.NewProc("PrjStartVirtualizing")
	prjStopVirtualizing     = projfsLib.NewProc("PrjStopVirtualizing")
	// Add more as needed: PrjMarkDirectoryAsPlaceholder, etc.
)

// ── Constants (HRESULTs, errors) ────────────────────────────────────────────

const (
	S_OK                     = 0
	ERROR_ACCESS_DENIED int32 = 5
	ERROR_READ_ONLY     int32 = 19
	ERROR_NOT_SUPPORTED int32 = 50
)

// ── Simplified callback structs (match Microsoft headers!) ──────────────────

type PrjCallbackData struct {
	ProcessId uint32
	// Real struct has many more fields — expand when needed
	// See: https://learn.microsoft.com/en-us/windows/win32/api/projectedfslib/ns-projectedfslib-prj_callback_data
}

type PrjCallbacks struct {
	StartDirectoryEnumeration   uintptr
	GetDirectoryEnumeration     uintptr
	EndDirectoryEnumeration     uintptr
	GetFileData                 uintptr
	// ... add more as you implement (at least 10-12 typically needed)
	// For minimal: at least these + dummies for others
}

// ── State ───────────────────────────────────────────────────────────────────

var (
	mu sync.Mutex

	authCache   = make(map[uint32]bool) // PID -> authorized (simpler than handle for now)
	realKeyPath = filepath.Join(os.Getenv("APPDATA"), "mysecure", "encrypted_id_rsa.bin")
	encKey      = deriveKey("test123") // CHANGE + make secure!
	decrypted   []byte                 // cached after first decrypt
)

func deriveKey(pass string) []byte {
	h := sha256.Sum256([]byte(pass))
	return h[:32] // AES-256 needs 32 bytes
}

// Load & decrypt real key (called once after auth)
func loadDecrypted() ([]byte, error) {
	data, err := os.ReadFile(realKeyPath)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("invalid encrypted file")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
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
	windows.QueryFullProcessImageName(h, 0, &buf[0], &size)
	return filepath.Base(syscall.UTF16ToString(buf[:size]))
}

// ── Core callback: GetFileData ──────────────────────────────────────────────

func getFileData(cb uintptr) uintptr {
	data := (*PrjCallbackData)(unsafe.Pointer(cb)) // unsafe cast — careful!

	pid := data.ProcessId
	proc := getProcessName(pid)
	log.Printf("GetFileData called by PID %d (%s)", pid, proc)

	if proc != "ssh.exe" && proc != "git.exe" {
		log.Println("Denied: not allowed process")
		return uintptr(ERROR_ACCESS_DENIED)
	}

	mu.Lock()
	defer mu.Unlock()

	if !authCache[pid] {
		fmt.Print("Enter password for id_rsa: ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		input := strings.TrimSpace(scanner.Text())

		// Simple check — replace with proper hash comparison
		if sha256.Sum256([]byte(input)) != sha256.Sum256([]byte("test123")) {
			log.Println("Wrong password")
			return uintptr(ERROR_ACCESS_DENIED)
		}

		var err error
		decrypted, err = loadDecrypted()
		if err != nil {
			log.Printf("Decrypt failed: %v", err)
			return uintptr(ERROR_ACCESS_DENIED)
		}

		authCache[pid] = true
		log.Println("Authorized for this process")
	}

	// Provide content (very basic — real code needs to handle offset/length)
	// For simplicity assume full read; expand for partial
	copy(unsafe.Slice((*byte)(unsafe.Pointer(uintptr(0))), len(decrypted)), decrypted) // TODO: real buffer handling!

	return S_OK
}

// Dummy callback (return read-only/not-supported)
func dummy(_ uintptr) uintptr {
	return uintptr(ERROR_NOT_SUPPORTED)
}

// ── Main ────────────────────────────────────────────────────────────────────

func main() {
	sshDir := filepath.Join(os.Getenv("USERPROFILE"), ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		log.Fatal(err)
	}

	callbacks := PrjCallbacks{
		GetFileData: syscall.NewCallback(getFileData),
		// Add dummies for required callbacks (at minimum):
		// StartDirectoryEnumeration: syscall.NewCallback(dummy),
		// etc. — see MS docs for full list
		// Without them, enumeration (dir .ssh) will fail/crash
	}

	rootPath, _ := syscall.UTF16PtrFromString(sshDir)

	var instanceID windows.GUID // will be filled by PrjStartVirtualizing

	hr, _, err := prjStartVirtualizing.Call(
		uintptr(unsafe.Pointer(rootPath)),
		uintptr(unsafe.Pointer(&callbacks)),
		0,                              // instanceContext (optional)
		0,                              // options
		uintptr(unsafe.Pointer(&instanceID)), // ← fixed: &instanceID
	)
	if int32(hr) < 0 {
		log.Fatalf("PrjStartVirtualizing failed: HRESULT 0x%08x, err %v", hr, err)
	}

	log.Printf("Virtualization started for %s (instance GUID: %s)", sshDir, instanceID.String())
	log.Println("Try: type %USERPROFILE%\\.ssh\\id_rsa   or   ssh -i ...")
	log.Println("Ctrl+C to stop")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	prjStopVirtualizing.Call(uintptr(unsafe.Pointer(&instanceID)))
	log.Println("Stopped.")
}