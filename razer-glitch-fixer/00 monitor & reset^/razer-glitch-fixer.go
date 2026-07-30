//go:build windows
// +build windows

// Copyright 2026 workturnedplay
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/workturnedplay/wincoe"
	"golang.org/x/sys/windows"
)

// Common Windows VK codes for media keys (consumer page)
var mediaKeys = map[uint16]string{
	0xB0: "MEDIA_NEXT_TRACK",
	0xB1: "MEDIA_PREV_TRACK",
	0xB2: "MEDIA_STOP",
	0xB3: "MEDIA_PLAY_PAUSE",
	0xAE: "VOLUME_UP",
	0xAF: "VOLUME_DOWN",
}

// Detection thresholds — tweak these
const (
	glitchWindow    = 1500 * time.Millisecond // look back this far
	glitchThreshold = 6                       // 6+ same key in the window = glitch
)

// ===================================================

var (
	mutex    sync.Mutex
	keyTimes = make(map[uint16][]time.Time) // per-key sliding window of timestamps
)

const WAIT_FOR_GLITCH_SECONDS = 10

func main() {
	runtime.LockOSThread()
	defer deinit()

	fmt.Println("Razer Glitch Fixer starting... (must run as Administrator to can usbreset the keyboard)")
	fmt.Printf("Waiting %d seconds for glitch to happen.\n", WAIT_FOR_GLITCH_SECONDS)

	go hookWorker()

	time.Sleep(WAIT_FOR_GLITCH_SECONDS * time.Second) // wait longer so that after reset the glitch might still happen(ie. anew) then we can reset again!

	deinit()
	time.Sleep(1 * time.Second) // so that the following is last msg

	if !wincoe.WaitAnyKeyIfInteractive() {
		logf("Didn't wait for keypress due to not an interactive/terminal.")
	}

	logf("main() finished.")
}

var (
	hookThreadId uint32
	kbdHook      windows.Handle
)

func hookWorker() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hookThreadId = windows.GetCurrentThreadId()

	// Install the keyboard hook via wincoe wrapper
	h, res := wincoe.SetWindowsHookEx(wincoe.WH_KEYBOARD_LL, keyboardProc, 0, 0)
	if res.Failed() {
		logf("Failed to install keyboard hook: %v", res.Err)
		return
	}
	kbdHook = h

	// Defer unhook (runs even on panic)
	defer func() {
		wincoe.UnhookWindowsHookEx(kbdHook)
		kbdHook = 0
		logf("Keyboard hook unhooked")
	}()

	// Private GetMessage loop — this is what makes the hook fire!
	var msg wincoe.MSG
	for {
		resMsg := wincoe.GetMessage(&msg, 0, 0, 0)
		if int32(resMsg.R1) <= 0 {
			logf("Hook worker thread received WM_QUIT(==0) or error(==-1) ret=%d, exiting and unhooking...", resMsg.R1)
			break // WM_QUIT
		}
		wincoe.TranslateMessage(&msg)
		wincoe.DispatchMessage(&msg)
	}

	logf("Hook worker done.")
}

func deinit() {
	if hookThreadId != 0 {
		// Send WM_QUIT (0x0012) directly to the hook thread's message queue
		wincoe.PostThreadMessage(hookThreadId, wincoe.WM_QUIT, 0, 0)
	}
	logf("Execution finished.")
}

func keyboardProc(nCode int32, wParam uintptr, lParam unsafe.Pointer) uintptr {
	kbd := (*wincoe.KBDLLHOOKSTRUCT)(lParam)

	// If nCode is less than zero, the hook procedure must pass the message to CallNextHookEx without further processing.
	if nCode < 0 {
		goto next
	}

	if nCode == wincoe.HC_ACTION {
		vk := uint16(kbd.VkCode) // truncate to uint16 to match your map key type

		// Only process media/consumer keys we care about
		name, isMedia := mediaKeys[vk]
		if !isMedia {
			goto next
		}
		logf("Media key (down or up): %s (VK=0x%02X)", name, vk)

		// ────────────────────────────────────────────────
		//  ↓  This is the migrated glitch detection logic  ↓
		// ────────────────────────────────────────────────

		mutex.Lock()
		now := time.Now()

		// Append current timestamp
		keyTimes[vk] = append(keyTimes[vk], now)

		// Remove timestamps older than the detection window
		windowStart := now.Add(-glitchWindow)
		for len(keyTimes[vk]) > 0 && keyTimes[vk][0].Before(windowStart) {
			keyTimes[vk] = keyTimes[vk][1:]
		}

		count := len(keyTimes[vk])
		mutex.Unlock()

		// Threshold check — tune glitchThreshold & glitchWindow to avoid false positives
		if count >= glitchThreshold {
			logf("!!! GLITCH DETECTED: %s (VK=0x%02X) pressed %d times in %.1fs",
				name, vk, count, glitchWindow.Seconds())

			// Trigger reset (non-blocking from hook perspective)
			go func() {
				if true {
					if err := resetRazerKeyboardViaClassInstaller(); err != nil {
						logf("Reset failed: %v", err)
					} else {
						logf("Keyboard successfully reset — glitch should be cleared.")
					}
				}
			}()

			// Clear history to prevent immediate re-trigger on next keys
			mutex.Lock()
			keyTimes[vk] = nil // or keyTimes[vk] = keyTimes[vk][:0] if you prefer to reuse slice
			mutex.Unlock()
		}
	}

next:
	return wincoe.CallNextHookEx(0, nCode, wParam, uintptr(lParam)).R1
}

// func isMediaKey(vk uint32) bool {
// 	switch vk {
// 	case 0xB0, 0xB1, 0xB2, 0xB3, // next, prev, stop, play/pause
// 		0xAE, 0xAF: // vol up/down
// 		return true
// 	}
// 	return false
// }

func logf(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	now := time.Now().Format("Mon Jan 2 15:04:05.000000000 MST 2006")
	finalMsg := fmt.Sprintf("[%s] %s\n", now, s)
	fmt.Print(finalMsg)
}

// ================== RESET USING DEVCON ==================

// resetRazerKeyboardViaDevCon finds the Razer device and restarts it using devcon64.exe (this works)
func resetRazerKeyboardViaDevCon() error {
	devices, err := listRazerDevices()
	if err != nil {
		return fmt.Errorf("failed to list Razer devices: %w", err)
	}

	if len(devices) == 0 {
		return fmt.Errorf("no Razer device (VID_1532&PID_0109) found")
	}

	// Prefer the composite USB device if available, otherwise any
	target := devices[0].InstanceID
	for _, d := range devices {
		if strings.Contains(d.InstanceID, "USB\\VID_1532&PID_0109") && !strings.Contains(d.InstanceID, "MI_") {
			target = d.InstanceID
			logf("Using composite USB device for reset: %s", target)
			break
		}
	}

	logf("Restarting Razer device: %s", target)

	// Call devcon64.exe (must be in the same folder as the exe)
	cmd := exec.Command("devcon64.exe", "restart", "@"+target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("devcon restart failed: %w", err)
	}

	logf("devcon restart command sent successfully")
	return nil
}

// listRazerDevices returns all devices matching VID_1532&PID_0109
func listRazerDevices() ([]Device, error) {
	h, res := wincoe.SetupDiGetClassDevs(
		nil,
		nil,
		0,
		wincoe.DIGCF_ALLCLASSES|wincoe.DIGCF_PRESENT,
	)
	if res.Failed() {
		return nil, res.Err
	}
	defer wincoe.SetupDiDestroyDeviceInfoList(h)

	var devices []Device

	for i := uint32(0); ; i++ {
		var data wincoe.SP_DEVINFO_DATA
		data.CbSize = uint32(unsafe.Sizeof(data))

		resEnum := wincoe.SetupDiEnumDeviceInfo(h, i, &data)
		if resEnum.Failed() {
			break
		}

		instance, _ := getInstanceID(h, &data)
		name, _ := getDeviceDesc(h, &data)

		if strings.Contains(instance, "VID_1532&PID_0109") {
			devices = append(devices, Device{
				InstanceID: instance,
				Name:       name,
			})
		}
	}

	return devices, nil
}

type Device struct {
	InstanceID string
	Name       string
}

func getInstanceID(h windows.Handle, data *wincoe.SP_DEVINFO_DATA) (string, error) {
	buf := make([]uint16, 512)
	var required uint32
	res := wincoe.SetupDiGetDeviceInstanceId(
		h,
		data,
		&buf[0],
		uint32(len(buf)),
		&required,
	)
	if res.Failed() {
		return "", res.Err
	}
	return windows.UTF16ToString(buf), nil
}

func getDeviceDesc(h windows.Handle, data *wincoe.SP_DEVINFO_DATA) (string, error) {
	buf := make([]byte, 512)
	var regType uint32
	var required uint32
	res := wincoe.SetupDiGetDeviceRegistryProperty(
		h,
		data,
		wincoe.SPDRP_DEVICEDESC,
		&regType,
		&buf[0],
		uint32(len(buf)),
		&required,
	)
	if res.Failed() {
		return "", res.Err
	}

	if required > 0 {
		slice := unsafe.Slice((*uint16)(unsafe.Pointer(&buf[0])), required/2)
		return windows.UTF16ToString(slice), nil
	}
	return "", nil
}

// resetRazerKeyboardViaClassInstaller is the pure-Go equivalent of "devcon restart" (this works)
func resetRazerKeyboardViaClassInstaller() error {
	devices, err := listRazerDevices()
	if err != nil {
		return fmt.Errorf("failed to list Razer devices: %w", err)
	}
	if len(devices) == 0 {
		return fmt.Errorf("no Razer device (VID_1532&PID_0109) found")
	}

	// Prefer composite USB device for real power cycle
	target := devices[0].InstanceID
	for _, d := range devices {
		if strings.Contains(d.InstanceID, "USB\\VID_1532&PID_0109") && !strings.Contains(d.InstanceID, "MI_") {
			target = d.InstanceID
			logf("Using USB composite device for reset: %s", target)
			break
		}
	}

	logf("Issuing class installer reset on: %s", target)

	if err := propertyChangeReset(target); err != nil {
		return fmt.Errorf("class installer reset failed: %w", err)
	}

	logf("Class installer reset sent successfully")
	return nil
}

// propertyChangeReset does what devcon "restart" does
func propertyChangeReset(instanceID string) error {
	h, res := wincoe.SetupDiGetClassDevs(
		nil,
		nil,
		0,
		wincoe.DIGCF_ALLCLASSES|wincoe.DIGCF_PRESENT,
	)
	if res.Failed() {
		return fmt.Errorf("SetupDiGetClassDevs failed: %w", res.Err)
	}
	defer wincoe.SetupDiDestroyDeviceInfoList(h)

	for i := uint32(0); ; i++ {
		var data wincoe.SP_DEVINFO_DATA
		data.CbSize = uint32(unsafe.Sizeof(data))

		resEnum := wincoe.SetupDiEnumDeviceInfo(h, i, &data)
		if resEnum.Failed() {
			break
		}

		instance, _ := getInstanceID(h, &data)
		if instance != instanceID {
			continue
		}

		// Found the device - now do the property change (DICS_PROPCHANGE)
		return doPropertyChange(h, &data)
	}

	return fmt.Errorf("device not found: %s", instanceID)
}

func doPropertyChange(h windows.Handle, devInfo *wincoe.SP_DEVINFO_DATA) error {
	// Prepare property change params
	var pcp wincoe.SP_PROPCHANGE_PARAMS
	pcp.ClassInstallHeader.CbSize = uint32(unsafe.Sizeof(pcp.ClassInstallHeader))
	pcp.ClassInstallHeader.InstallFunction = wincoe.DIF_PROPERTYCHANGE
	pcp.StateChange = wincoe.DICS_PROPCHANGE // This is what devcon uses for "restart"
	pcp.Scope = wincoe.DICS_FLAG_GLOBAL

	// Set the class install params
	res1 := wincoe.SetupDiSetClassInstallParams(
		h,
		devInfo,
		&pcp,
		uint32(unsafe.Sizeof(pcp)),
	)
	if res1.Failed() {
		return fmt.Errorf("SetupDiSetClassInstallParams failed: %w", res1.Err)
	}

	// Call the class installer to perform the reset
	res2 := wincoe.SetupDiCallClassInstaller(
		wincoe.DIF_PROPERTYCHANGE,
		h,
		devInfo,
	)
	if res2.Failed() {
		who := "SetupDiCallClassInstaller (DIF_PROPERTYCHANGE)"
		// Defensive: err can be nil in some syscall edge cases
		if res2.Err != nil {
			if errors.Is(res2.Err, windows.ERROR_ACCESS_DENIED) {
				return fmt.Errorf("%s failed: %w: administrative privileges required", who, res2.Err)
			}
			return fmt.Errorf("%s failed: %w", who, res2.Err)
		}

		// Extremely defensive fallback
		return fmt.Errorf("SetupDiCallClassInstaller failed: unknown error")
	}

	logf("Class installer reset (via DIF_PROPERTYCHANGE / DICS_PROPCHANGE) issued successfully.")
	return nil
}
