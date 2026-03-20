package main

import (
	"fmt"
	"runtime"
	"syscall"
	"time"
	
)

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
)

func getAsyncKeyState(vk uint32) (int16, error) {
	ret, _, lastErr := procGetAsyncKeyState.Call(uintptr(vk))
	return int16(ret), lastErr
}

func main() {
	// Set up a high-frequency polling loop
	pollingFrequency := 2000 // Hz
	sleepTime := time.Duration(1000000000/pollingFrequency) * time.Nanosecond

	for {
		// Poll GetAsyncKeyState for all keys (256 possible keys)
		for vk := uint32(0); vk < 256; vk++ {
			keyState, err := getAsyncKeyState(vk)
			if err != nil {
				//fmt.Printf("Error getting key state for vk %d: %v\n", vk, err)
				continue
			}

			// Check the key state and transition bit
			if int32(keyState)&0x8000 != 0 { // Key down
				fmt.Printf("Key %d down\n", vk)
			} else { // Key up
				//fmt.Printf("Key %d up\n", vk)
			}
		}

		// Sleep until the next poll
		time.Sleep(sleepTime)
		runtime.Gosched() // Allow other goroutines to run
	}
}
