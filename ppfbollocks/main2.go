package main

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/winfsp/cgofuse/fuse"
	"golang.org/x/sys/windows"
	// For GUI prompt: "github.com/lxn/win"
)

// ── Constants ───────────────────────────────────────────────────────────────

const (
	fileName = "id_rsa" // Emulated file name
	rootDir  = "/.ssh"  // Mount point (e.g., Z:\.ssh, but we use in-memory path)
)

var (
	realKeyPath = filepath.Join(os.Getenv("APPDATA"), "mysecure", "encrypted_id_rsa.bin")
	encKey      = deriveKey("test123") // CHANGE THIS!
	decrypted   []byte
	mu          sync.Mutex
	authPIDs    = make(map[int64]bool) // PID -> authorized
)

// ── Secure Open Real File (exclusive, with ACLs) ────────────────────────────

func secureOpenRealFile() (*os.File, error) {
	// First, set ACLs to only allow current user
	err := setACLs(realKeyPath)
	if err != nil {
		return nil, err
	}

	// Open exclusively (dwShareMode=0)
	pathPtr, _ := syscall.UTF16PtrFromString(realKeyPath)
	h, err := windows.CreateFile(pathPtr, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), realKeyPath), nil
}

func setACLs(path string) error {
	// Get current user SID
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token)
	if err != nil {
		return err
	}
	userInfo, err := token.GetTokenUser()
	if err != nil {
		return err
	}

	// Create DACL allowing only this SID read access
	// Simplified: Deny everyone else
	// Use windows.SetNamedSecurityInfo (manual call)
	// For full: See https://pkg.go.dev/golang.org/x/sys/windows#SetNamedSecurityInfo
	// Placeholder: Implement with explicit denies if needed
	return nil // TODO: Full ACL setup
}

func deriveKey(pass string) []byte {
	h := sha256.Sum256([]byte(pass))
	return h[:32]
}

func loadDecrypted() ([]byte, error) {
	f, err := secureOpenRealFile()
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := os.ReadFile(realKeyPath) // But since exclusive, read here
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
		return nil, fmt.Errorf("invalid ciphertext")
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// ── MemFS Implementation ────────────────────────────────────────────────────

type MemFS struct {
	fuse.FileSystemBase
}

func (self *MemFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	if path == "/" {
		stat.Mode = fuse.S_IFDIR | 0755
		return 0
	} else if path == "/"+fileName {
		stat.Mode = fuse.S_IFREG | 0400 // Read-only
		stat.Size = int64(len(decrypted)) // Assume loaded
		return 0
	}
	return -fuse.ENOENT
}

func (self *MemFS) Open(path string, flags int) (errc int, fh uint64) {
	if path != "/"+fileName {
		return -fuse.ENOENT, ^uint64(0)
	}
	if flags&fuse.O_ANYWRITE != 0 {
		return -fuse.EACCES, ^uint64(0)
	}

	ctx := fuse.GetContext()
	pid := ctx.Pid
	proc := getProcessName(uint32(pid))
	log.Printf("Open by PID %d (%s)", pid, proc)

	if proc != "ssh.exe" && proc != "git.exe" {
		return -fuse.EPERM, ^uint64(0)
	}

	mu.Lock()
	defer mu.Unlock()

	if !authPIDs[pid] {
		// Prompt
		fmt.Print("Enter password: ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		input := strings.TrimSpace(scanner.Text())

		// TODO: GUI with lxn/win.MessageBox + input

		if sha256.Sum256([]byte(input)) != sha256.Sum256([]byte("test123")) {
			log.Println("Wrong password")
			return -fuse.EACCES, ^uint64(0)
		}

		var err error
		decrypted, err = loadDecrypted()
		if err != nil {
			log.Printf("Load failed: %v", err)
			return -fuse.EIO, ^uint64(0)
		}

		authPIDs[pid] = true
	}

	return 0, 0 // fh=0 for simplicity (in-memory)
}

func (self *MemFS) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	if path != "/"+fileName {
		return -fuse.ENOENT
	}
	endofst := ofst + int64(len(buff))
	if endofst > int64(len(decrypted)) {
		endofst = int64(len(decrypted))
	}
	if endofst <= ofst {
		return 0
	}
	n = copy(buff, decrypted[ofst:endofst])
	return
}

func (self *MemFS) Readdir(path string, fill func(name string, stat *fuse.Stat_t, ofst int64) bool, ofst int64, fh uint64) (errc int) {
	fill(".", nil, 0)
	fill("..", nil, 0)
	stat := &fuse.Stat_t{Mode: fuse.S_IFREG | 0400, Size: int64(len(decrypted))}
	fill(fileName, stat, 0)
	return 0
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

// ── Main ────────────────────────────────────────────────────────────────────

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: memfs MOUNTPOINT")
	}
	mountpoint := os.Args[1]

	fs := &MemFS{}
	host := fuse.NewFileSystemHost(fs)
	host.Mount(mountpoint, nil) // Mount options nil for default
}