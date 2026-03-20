package main

import (
	"fmt"
	"syscall"
	"time"
	//"unsafe"
)

const (
	POLLING_FREQUENCY = 1000 // 1 kHz
)

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
)

func getAsyncKeyState(vkCode uint16) (state uint16) {
	r1, _, _ := procGetAsyncKeyState.Call(uintptr(vkCode))
	return uint16(r1)
}

func pollKeys() {
	keysToPoll := []uint16{
		'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
		'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 0x20, 0x0D, 0x08, // VK_SPACE, VK_RETURN, VK_BACK
	}

	ticker := time.NewTicker(time.Duration(1000/POLLING_FREQUENCY) * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		for _, key := range keysToPoll {
			state := getAsyncKeyState(key)
			if state&0x8000 != 0 {
				fmt.Printf("Key %X is down\n", key)
			}
		}
	}
}

func main() {
	fmt.Println("Starting key polling...")
	pollKeys()
}
