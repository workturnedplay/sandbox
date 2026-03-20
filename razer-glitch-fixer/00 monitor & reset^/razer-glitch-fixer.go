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
	//"bytes"
	//"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
	//hook "github.com/robotn/gohook"
	//"github.com/workturnedplay/wincoe"
	"errors"
	"golang.org/x/sys/windows"
	"unsafe"
)

// ================== CONFIGURATION ==================
// // Change these to match YOUR Razer keyboard
// const (
// 	//deviceInstanceID = `USB\VID_1532&PID_0220\6&1a2b3c4d5e6f7g8h9i0j&0&0` // ← FIND YOURS (see below)
// 	resetDelay = 2 * time.Second // how long to wait between disable and enable
// )

// Common Windows VK codes for media keys (consumer page)
var mediaKeys = map[uint16]string{
	0xB0: "MEDIA_NEXT_TRACK",
	0xB1: "MEDIA_PREV_TRACK",
	0xB2: "MEDIA_STOP",
	0xB3: "MEDIA_PLAY_PAUSE",
	0xAE: "VOLUME_UP",
	0xAF: "VOLUME_DOWN",
}

type SP_DEVINFO_DATA struct {
	CbSize    uint32
	ClassGuid windows.GUID
	DevInst   uint32
	Reserved  uintptr
}

var (
	// cfgmgr32 = windows.NewLazySystemDLL("cfgmgr32.dll")
	user32 = windows.NewLazySystemDLL("user32.dll")

	// procCM_Get_DevNode_Status = cfgmgr32.NewProc("CM_Get_DevNode_Status")
	// procCM_Disable_DevNode    = cfgmgr32.NewProc("CM_Disable_DevNode")
	// procCM_Enable_DevNode     = cfgmgr32.NewProc("CM_Enable_DevNode")
	// procCM_Locate_DevNodeW    = cfgmgr32.NewProc("CM_Locate_DevNodeW")

	procSetWindowsHookEx    = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")

	procGetMessage       = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")

	procPostThreadMessage = user32.NewProc("PostThreadMessageW")

	procSetupDiGetDeviceInstanceId       = setupapi.NewProc("SetupDiGetDeviceInstanceIdW")
	procSetupDiGetClassDevs              = setupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiGetDeviceRegistryProperty = setupapi.NewProc("SetupDiGetDeviceRegistryPropertyW")

	procSetupDiSetClassInstallParams = setupapi.NewProc("SetupDiSetClassInstallParamsW")
	procSetupDiCallClassInstaller    = setupapi.NewProc("SetupDiCallClassInstaller")
)

const (
	CM_GETIDLIST_FILTER_PRESENT = 0x00000000
	CM_GETIDLIST_FILTER_CLASS   = 0x00000001
	CM_GETIDLIST_FILTER_SERVICE = 0x00000002
	// ...
	CR_SUCCESS         = 0x00000000 // 0
	CR_NO_SUCH_DEVNODE = 0x00000019 // 25 decimal
	CR_NEED_RESTART    = 0x33       // 55
	CR_INVALID_FLAG    = 0x0D       // 13
	CR_ACCESS_DENIED   = 0x21       // 33 decimal
	//This error indicates that the device node (devInst handle) is already open by another caller / component,
	// and that holder has it in a state that prevents the disable operation from proceeding cleanly.
	CR_OPEN_HANDLE = 0x00000028 // 40 decimal
)

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

// var globalRazerInstanceID string

// // Global or init-time variable
// var deviceInstanceID string

// // Call this once at startup (in main or init)
// func initRazerInstanceID() error {
// 	// Query all HID devices, filter by VID_1532&PID_0109 (your keyboard model)
// 	// Output as JSON for easy parsing
// 	psScript := `
// $devices = Get-PnpDevice -Class HIDClass -PresentOnly |
//     Where-Object { $_.InstanceId -match 'VID_1532&PID_0109' } |
//     Select-Object -Property InstanceId, FriendlyName, Status |
//     ConvertTo-Json -Compress

// if ($devices) {
//     $devices
// } else {
//     "[]"
// }
// `

// 	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", psScript)
// 	var out bytes.Buffer
// 	cmd.Stdout = &out
// 	cmd.Stderr = &out // capture errors too

// 	if err := cmd.Run(); err != nil {
// 		return fmt.Errorf("failed to query PnP devices: %w\nOutput: %s", err, out.String())
// 	}

// 	output := strings.TrimSpace(out.String())
// 	if output == "[]" {
// 		return fmt.Errorf("no HID device found matching VID_1532&PID_0109")
// 	}

// 	// Parse JSON array (even if single item)
// 	type dev struct {
// 		InstanceId   string `json:"InstanceId"`
// 		FriendlyName string `json:"FriendlyName"`
// 		Status       string `json:"Status"`
// 	}

// 	var devs []dev
// 	if err := json.Unmarshal([]byte(output), &devs); err != nil {
// 		return fmt.Errorf("failed to parse PnP JSON: %w\nRaw: %s", err, output)
// 	}

// 	if len(devs) == 0 {
// 		return fmt.Errorf("empty device list after filter")
// 	}

// 	if len(devs) > 1 {
// 		logf("Warning: multiple matches found — using first one. Full list:")
// 		for _, d := range devs {
// 			logf("  - %s (%s)", d.InstanceId, d.FriendlyName)
// 		}
// 	}

// 	// Inside the loop after parsing devs
// 	for _, d := range devs {
// 		if strings.Contains(strings.ToLower(d.FriendlyName), "consumer") ||
// 			strings.Contains(d.InstanceId, "MI_01&COL01") {
// 			deviceInstanceID = d.InstanceId
// 			logf("Selected preferred consumer/media interface: %s (%s)", deviceInstanceID, d.FriendlyName)
// 			return nil // or break and return
// 		}
// 	}

// 	// Prefer the first one that looks like a keyboard (or log all)
// 	// You can add more logic: check FriendlyName contains "Keyboard", or Status == "OK"
// 	deviceInstanceID = devs[0].InstanceId
// 	logf("Fallback picked first keyboard InstanceId: %s (Name: %s, Status: %s)",
// 		deviceInstanceID, devs[0].FriendlyName, devs[0].Status)

// 	return nil
// }

func main() {
	runtime.LockOSThread()
	defer deinit()

	fmt.Println("Razer Glitch Fixer starting... (must run as Administrator to can usbreset the keyboard)")
	//fmt.Println("Press Ctrl+Shift+Q to quit cleanly.")

	// if err := initRazerInstanceID(); err != nil {
	// 	logf("Fatal: %v — cannot continue without InstanceId", err)
	// 	return // or os.Exit(1)
	// }

	// // Start low-level hook (captures EVERY keyboard event, including phantom media keys)
	// evChan := hook.Start()
	// defer hook.End()

	// globalRazerInstanceID = findRazerKeyboardInstanceID()
	// if globalRazerInstanceID == "" {
	// 	logf("Cannot auto-detect Razer keyboard — exiting or falling back to manual config")
	// 	// os.Exit(1) or prompt user
	// }

	go hookWorker()

	// devInst, err := getDevInstFromID(instanceID)
	// if err != nil { ... }

	// if err := disableDevice(devInst); err != nil { ... }
	// time.Sleep(1800 * time.Millisecond)
	// if err := enableDevice(devInst); err != nil { ... }

	// Spawn the detector in background
	// go detectGlitchLoop(evChan)

	// // Block until quit hotkey
	// s := hook.Start()
	// <-hook.Process(s) // this also handles the registered quit hotkey

	time.Sleep(10 * time.Second) // wait longer so that after reset the glitch might still happen(ie. anew) then we can reset again!

	deinit()
	logf("Press Enter to exit... TODO: use any key and clrbuf before&after")
	var dummy string
	_, _ = fmt.Scanln(&dummy)

	logf("main() finished.")
}

// func resetRazerKeyboard() error { // last good
// 	id := globalRazerInstanceID // set at startup
// 	if id == "" {
// 		logf("No Razer instance known, thus cannot reset.")
// 		return fmt.Errorf("no Razer keyboard instance ID found (auto-detect failed)")
// 	}

// 	devInst, err := getDevInstFromID(id)
// 	if err != nil {
// 		logf("Cannot locate devnode: %v", err)
// 		return fmt.Errorf("CM_Locate_DevNodeW failed: %w", err)
// 	}

// 	logf("Disabling Razer keyboard node...")
// 	if err := disableDevice(devInst); err != nil {
// 		logf("Disable failed: %v", err)
// 		return fmt.Errorf("CM_Disable_DevNode failed: %w", err)
// 	}

// 	//time.Sleep(2 * time.Second)
// 	const DN_STARTED = 0x00000008 // devnode is started/running

// 	for i := 0; i < 10; i++ { // poll up to ~5s
// 		var status, problem uint32
// 		r, _, _ := procCM_Get_DevNode_Status.Call(
// 			uintptr(unsafe.Pointer(&status)),
// 			uintptr(unsafe.Pointer(&problem)),
// 			devInst, 0,
// 		)
// 		if r == CR_SUCCESS && (status&DN_STARTED) == 0 {
// 			logf("Device now appears not started (status=0x%X)", status)
// 			break
// 		}
// 		time.Sleep(500 * time.Millisecond)
// 		logf("waiting 500ms more...")
// 	}

// 	logf("Re-enabling...")
// 	if err := enableDevice(devInst); err != nil {
// 		logf("Enable failed: %v", err)
// 		return fmt.Errorf("CM_Enable_DevNode failed: %w", err)
// 	} else {
// 		logf("Reset complete")
// 		return nil
// 	}
// }

// // resetRazerKeyboard tries to clear the glitch by poking the device state.
// // It prefers the consumer control interface (where media keys live).
// func resetRazerKeyboard() error {
// 	// Prefer consumer interface if we have it
// 	targetID := deviceInstanceID
// 	if strings.Contains(deviceInstanceID, "MI_01&COL01") ||
// 		strings.Contains(strings.ToLower(deviceInstanceID), "consumer") {
// 		logf("Using preferred consumer/media interface: %s", targetID)
// 	} else {
// 		// Try to find consumer interface from the list we already queried
// 		// (we can improve this later if needed)
// 		logf("Using fallback interface: %s", targetID)
// 	}

// 	logf("Attempting glitch reset on: %s", targetID)

// 	// 1. Try PowerShell first (often more forgiving on HID devices)
// 	logf("Trying PowerShell reset...")
// 	if err := resetViaPowerShell(targetID); err == nil {
// 		logf("PowerShell reset succeeded")
// 		return nil
// 	} else {
// 		logf("PowerShell reset failed: %v", err)
// 	}

// 	// 2. Fallback to CM path (your existing CM_Disable_DevNode logic)
// 	logf("Trying CM (cfgmgr32) reset as fallback...")
// 	if err := resetViaCM(targetID); err == nil {
// 		logf("CM reset succeeded")
// 		return nil
// 	} else {
// 		logf("CM reset also failed: %v", err)
// 	}

// 	// 3. Even if both "failed", the glitch is often cleared anyway (as you saw)
// 	logf("Both reset methods reported failure, but glitch was likely cleared by the attempt itself (common Razer firmware behavior)")
// 	return nil // return nil so we don't spam-reset
// }

// // Helper: PowerShell version (what you already had in resetKeyboard)
// func resetViaPowerShell(id string) error {
// 	// Disable
// 	psCmd := fmt.Sprintf(`$ErrorActionPreference = "Stop"; Get-PnpDevice -InstanceId "%s" | Disable-PnpDevice -Confirm:$false`, id)
// 	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", psCmd)
// 	out, err := cmd.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("PowerShell disable failed: %v\nOutput: %s", err, strings.TrimSpace(string(out)))
// 	}

// 	time.Sleep(2 * time.Second)

// 	// Enable
// 	psCmd = fmt.Sprintf(`$ErrorActionPreference = "Stop"; Get-PnpDevice -InstanceId "%s" | Enable-PnpDevice -Confirm:$false`, id)
// 	cmd = exec.Command("powershell.exe", "-NoProfile", "-Command", psCmd)
// 	out, err = cmd.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("PowerShell enable failed: %v\nOutput: %s", err, strings.TrimSpace(string(out)))
// 	}

// 	return nil
// }

// // Helper: Your existing CM version (wrapped cleanly)
// func resetViaCM(id string) error { // doesn't work
// 	devInst, err := getDevInstFromID(id)
// 	if err != nil {
// 		return fmt.Errorf("CM_Locate_DevNodeW failed: %w", err)
// 	}

// 	logf("CM: Disabling devnode 0x%X...", devInst)
// 	if err := disableDevice(devInst); err != nil {
// 		return fmt.Errorf("CM disable failed: %w", err)
// 	}

// 	// Optional poll (you already had this)
// 	const DN_STARTED = 0x00000008
// 	for i := 0; i < 8; i++ {
// 		var status, problem uint32
// 		r, _, _ := procCM_Get_DevNode_Status.Call(
// 			uintptr(unsafe.Pointer(&status)),
// 			uintptr(unsafe.Pointer(&problem)),
// 			devInst, 0,
// 		)
// 		if r == CR_SUCCESS && (status&DN_STARTED) == 0 {
// 			logf("CM: Device now not started (status=0x%X)", status)
// 			break
// 		}
// 		time.Sleep(400 * time.Millisecond)
// 	}

// 	logf("CM: Re-enabling devnode...")
// 	if err := enableDevice(devInst); err != nil {
// 		return fmt.Errorf("CM enable failed: %w", err)
// 	}

// 	logf("CM reset completed")
// 	return nil
// }

// // getParentInstanceID returns the parent USB composite device (the one that actually controls power)
// func getParentInstanceID(childID string) (string, error) {
// 	ps := fmt.Sprintf(`
// $ErrorActionPreference = "Stop"
// $dev = Get-PnpDevice -InstanceId "%s" -ErrorAction SilentlyContinue
// if ($dev -and $dev.Parent) {
//     $parent = Get-PnpDevice -InstanceId $dev.Parent
//     $parent.InstanceId
// } else {
//     ""
// }
// `, childID)

// 	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", ps)
// 	out, err := cmd.CombinedOutput()
// 	if err != nil {
// 		return "", fmt.Errorf("parent query failed: %v\nOutput: %s", err, string(out))
// 	}

// 	parent := strings.TrimSpace(string(out))
// 	if parent == "" {
// 		return "", fmt.Errorf("no parent found")
// 	}
// 	logf("Found parent USB composite device: %s", parent)
// 	return parent, nil
// }

// // resetRazerKeyboard — now uses parent for real power cycle (doesn't work)
// func resetRazerKeyboard() error {
// 	id := globalRazerInstanceID
// 	if id == "" {
// 		logf("No instance ID known")
// 		return fmt.Errorf("no Razer instance ID")
// 	}

// 	// 1. Try parent first (this is what actually cuts power)
// 	parentID, err := getParentInstanceID(id)
// 	if err == nil && parentID != "" {
// 		logf("Using PARENT for full power cycle: %s", parentID)
// 		if err := resetViaPowerShell(parentID); err == nil {
// 			logf("Parent reset succeeded — full power cycle should have happened")
// 			return nil
// 		}
// 		logf("Parent reset failed, falling back to child")
// 	}

// 	// 2. Fallback to child (consumer interface)
// 	logf("Using child interface as fallback: %s", id)
// 	if err := resetViaPowerShell(id); err == nil {
// 		logf("Child reset succeeded")
// 		return nil
// 	}

// 	logf("Both parent and child reset failed — glitch probably still there")
// 	return fmt.Errorf("reset failed")
// }

// // Helper: PowerShell disable/enable (most reliable for parent) (doesn't work)
// func resetViaPowerShell(id string) error {
// 	// Disable
// 	psCmd := fmt.Sprintf(`$ErrorActionPreference = "Stop"; Get-PnpDevice -InstanceId "%s" | Disable-PnpDevice -Confirm:$false`, id)
// 	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", psCmd)
// 	out, err := cmd.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("disable failed: %v\nOutput: %s", err, strings.TrimSpace(string(out)))
// 	}

// 	time.Sleep(3 * time.Second) // longer wait for real power cycle

// 	// Enable
// 	psCmd = fmt.Sprintf(`$ErrorActionPreference = "Stop"; Get-PnpDevice -InstanceId "%s" | Enable-PnpDevice -Confirm:$false`, id)
// 	cmd = exec.Command("powershell.exe", "-NoProfile", "-Command", psCmd)
// 	out, err = cmd.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("enable failed: %v\nOutput: %s", err, strings.TrimSpace(string(out)))
// 	}

// 	logf("PowerShell reset on %s completed", id)
// 	return nil
// }

// // Returns the Device Instance ID (e.g. "USB\\VID_1532&PID_XXXX\\...") or "" if not found
// func findRazerKeyboardInstanceID() string {
// 	// We could enumerate via SetupDiGetClassDevs(GUID_DEVINTERFACE_HID) but it's ~150 lines.
// 	// Simpler heuristic: look in "HKEY_LOCAL_MACHINE\SYSTEM\CurrentControlSet\Enum\USB" for VID_1532 keyboards

// 	// For simplicity we use a quick-and-dirty WMI-like approach via registry (not perfect but works on most systems)
// 	// Better would be full SetupAPI enumeration — left as exercise or use 3rd party if needed.

// 	// Placeholder — real implementation would walk Enum\HID or Enum\USB
// 	// For now assume you can get candidate paths from registry or hard-code temporarily
// 	candidates := []string{
// 		`USB\VID_1532&PID_0220`, // BlackWidow example — change to your model
// 		`USB\VID_1532&PID_0282`, // common Razer keyboard
// 		// add more known PIDs if you have multiple Razer devices
// 	}

// 	for _, prefix := range candidates {
// 		// Walk Enum\USB or Enum\HID — pseudo-code
// 		// In real code use registry.Walk or syscall to enumerate subkeys
// 		// Here we just return first match — improve later
// 		return prefix + `\6&something&0&0` // ← placeholder, replace with real logic
// 	}

// 	logf("No Razer keyboard found via heuristic")
// 	return ""
// }

var (
	setupapi = windows.NewLazySystemDLL("setupapi.dll")

	//procSetupDiGetClassDevsW         = setupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInfo = setupapi.NewProc("SetupDiEnumDeviceInfo")
	//procSetupDiGetDeviceInstanceIdW  = setupapi.NewProc("SetupDiGetDeviceInstanceIdW")
	procSetupDiDestroyDeviceInfoList = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
)

/*
Yes — the GUID is cryptic because it's a binary CLSID (not a string), used by SetupAPI to identify the device interface
class "Human Interface Devices" (keyboards, mice, gamepads, consumer controls, etc.).
It's defined as {4D1E55B2-F16F-11CF-88CB-001111000030}
The hex bytes are just the wire-format representation (Microsoft chose this format decades ago for COM-style GUIDs).
*/
var GUID_DEVINTERFACE_HID = windows.GUID{
	Data1: 0x4D1E55B2,
	Data2: 0xF16F,
	Data3: 0x11CF,
	Data4: [8]byte{0x88, 0xCB, 0x00, 0x11, 0x11, 0x00, 0x00, 0x30},
}

const (
	DIGCF_DEVICEINTERFACE = 0x00000010
	INVALID_HANDLE_VALUE  = ^uintptr(0) // -1
)

const (
	DIGCF_ALLCLASSES = 0x00000004
	DIGCF_PRESENT    = 0x00000002
	SPDRP_DEVICEDESC = 0x00000000
)

// // findRazerKeyboardInstanceID returns the first HID device instance ID
// // matching VID_1532 & PID_0109 (your exact Razer keyboard).
// // Returns "" if none found.
// func findRazerKeyboardInstanceID() string {
// 	logf("Starting HID enumeration for VID_1532&PID_0109...")

// 	// Get a list of all present HID devices
// 	hDevInfo, _, _ := procSetupDiGetClassDevsW.Call(
// 		uintptr(unsafe.Pointer(&GUID_DEVINTERFACE_HID)),
// 		0,
// 		0,
// 		DIGCF_PRESENT|DIGCF_DEVICEINTERFACE,
// 	)
// 	if hDevInfo == INVALID_HANDLE_VALUE {
// 		logf("SetupDiGetClassDevsW failed")
// 		return ""
// 	}
// 	defer procSetupDiDestroyDeviceInfoList.Call(hDevInfo)

// 	var devInfoData struct {
// 		cbSize    uint32
// 		classGuid windows.GUID
// 		devInst   uint32
// 		reserved  uintptr
// 	}
// 	devInfoData.cbSize = uint32(unsafe.Sizeof(devInfoData))

// 	for i := uint32(0); ; i++ {
// 		r, _, _ := procSetupDiEnumDeviceInfo.Call(
// 			hDevInfo,
// 			uintptr(i),
// 			uintptr(unsafe.Pointer(&devInfoData)),
// 		)
// 		if r == 0 {
// 			break // no more devices
// 		}

// 		// Get the full instance ID (e.g. HID\VID_1532&PID_0109&MI_00\7&xxxx&0&0000)
// 		buf := make([]uint16, 512)
// 		var needed uint32
// 		r, _, _ = procSetupDiGetDeviceInstanceIdW.Call(
// 			hDevInfo,
// 			uintptr(unsafe.Pointer(&devInfoData)),
// 			uintptr(unsafe.Pointer(&buf[0])),
// 			uintptr(len(buf)),
// 			uintptr(unsafe.Pointer(&needed)),
// 		)
// 		if r == 0 {
// 			continue
// 		}

// 		instanceID := windows.UTF16ToString(buf[:needed-1]) // -1 because NULL terminator

// 		// my Razer keyboard with the glitch: HID\VID_1532&PID_0109&MI_00\7&8F9FD76&0&0000
// 		/*
// 			Breakdown:
// 			HID\ → class/enumerator
// 			VID_1532&PID_0109 → vendor/product (Razer-specific model)
// 			&MI_00 → Multi-Interface 00 (your keyboard exposes multiple HID interfaces; 00 is usually the main keyboard one)
// 			\7&8F9FD76&0&0000 → instance-specific path:
// 			7&8F9FD76 = composite device instance ID (generated by Windows, changes on USB port change / driver reinstall / hardware move)
// 			&0&0000 = interface/instance index
// 		*/
// 		// Filter only by VID&PID (instance suffix changes)
// 		if strings.Contains(instanceID, `VID_1532&PID_0109`) {
// 			logf("FOUND matching Razer keyboard: %s", instanceID)
// 			return instanceID
// 		}
// 	}

// 	logf("No Razer keyboard (VID_1532&PID_0109) found in HID devices")
// 	return ""
// }

// func detectGlitchLoop(evChan chan hook.Event) {
// 	//for ev := range evChan {
// 	// We only care about KeyDown events (glitch usually spams press)
// 	// if ev.Kind != hook.KeyDown {
// 	// 	continue
// 	// }

// 	// Is this a media key we care about?
// 	name, isMedia := mediaKeys[ev.Keycode]
// 	if !isMedia {
// 		continue
// 	}

// 	mutex.Lock()
// 	now := time.Now()
// 	keyTimes[ev.Keycode] = append(keyTimes[ev.Keycode], now)

// 	// Prune old timestamps
// 	windowStart := now.Add(-glitchWindow)
// 	for i := 0; i < len(keyTimes[ev.Keycode]); i++ {
// 		if keyTimes[ev.Keycode][i].Before(windowStart) {
// 			keyTimes[ev.Keycode] = keyTimes[ev.Keycode][i+1:]
// 			break
// 		}
// 	}

// 	count := len(keyTimes[ev.Keycode])
// 	mutex.Unlock()

// 	if count >= glitchThreshold {
// 		fmt.Printf("!!! GLITCH DETECTED: %s pressed %d times in %.1fs → RESETTING KEYBOARD\n",
// 			name, count, glitchWindow.Seconds())
// 		if err := resetRazerKeyboard(); err != nil {
// 			fmt.Printf("Reset failed: %v\n", err)
// 		} else {
// 			fmt.Println("Keyboard successfully reset — glitch cleared.")
// 		}
// 		// Clear the history so we don't spam-reset
// 		mutex.Lock()
// 		keyTimes[ev.Keycode] = nil
// 		mutex.Unlock()
// 	}
// 	// }
// }

// func resetKeyboard() error { // doesn't work
// 	// Disable
// 	psCmd := fmt.Sprintf(`$ErrorActionPreference = "Stop"; Get-PnpDevice -InstanceId "%s" | Disable-PnpDevice -Confirm:$false`, deviceInstanceID)
// 	disableCmd := exec.Command("powershell.exe", "-NoProfile", "-Command", psCmd)
// 	// disableCmd := exec.Command("powershell.exe", "-NoProfile", "-Command",
// 	// 	fmt.Sprintf(`Get-PnpDevice -InstanceId "%s" | Disable-PnpDevice -Confirm:$false`, deviceInstanceID))

// 	out, err := disableCmd.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("disable failed: %v\nOutput: %s", err, strings.TrimSpace(string(out)))
// 	}

// 	time.Sleep(resetDelay)

// 	// Enable
// 	psCmd = fmt.Sprintf(`$ErrorActionPreference = "Stop"; Get-PnpDevice -InstanceId "%s" | Enable-PnpDevice -Confirm:$false`, deviceInstanceID)
// 	enableCmd := exec.Command("powershell.exe", "-NoProfile", "-Command", psCmd)
// 	// enableCmd := exec.Command("powershell.exe", "-NoProfile", "-Command",
// 	// 	fmt.Sprintf(`Get-PnpDevice -InstanceId "%s" | Enable-PnpDevice -Confirm:$false`, deviceInstanceID))

// 	out, err = enableCmd.CombinedOutput()
// 	if err != nil {
// 		return fmt.Errorf("enable failed: %v\nOutput: %s", err, strings.TrimSpace(string(out)))
// 	}

// 	return nil
// }

// // devInst is the DEVINST handle obtained via CM_Locate_DevNodeW
// func disableDevice(devInst uintptr) error {
// 	r, _, _ := procCM_Disable_DevNode.Call(devInst, 0)
// 	if r != CR_SUCCESS {
// 		return fmt.Errorf("CM_Disable_DevNode failed: %d", r)
// 	}
// 	return nil
// }

// func disableDevice(devInst uintptr) error {
// 	const maxRetries = 3
// 	for attempt := 1; attempt <= maxRetries; attempt++ {
// 		r, _, err := procCM_Disable_DevNode.Call(devInst, 0) // start with no persist
// 		if r == CR_SUCCESS {
// 			logf("Disable succeeded on attempt %d", attempt)
// 			return nil
// 		}

// 		logf("Disable attempt %d failed: ret=0x%X (%d decimal) err=%v", attempt, r, r, err)

// 		if r == CR_OPEN_HANDLE {
// 			logf("CR_OPEN_HANDLE detected — waiting 1s before retry (close Synapse/other apps?)")
// 			time.Sleep(1 * time.Second)
// 			continue
// 		}

// 		// For other errors, bail early
// 		return fmt.Errorf("CM_Disable_DevNode failed after %d attempts: 0x%X (%d)", attempt, r, r)
// 	}
// 	return fmt.Errorf("CM_Disable_DevNode failed after %d retries (persistent open handle?)", maxRetries)
// }

// func enableDevice(devInst uintptr) error {
// 	r, _, _ := procCM_Enable_DevNode.Call(devInst, 0)
// 	if r != CR_SUCCESS {
// 		return fmt.Errorf("CM_Enable_DevNode failed: %d", r)
// 	}
// 	return nil
// }

// const CM_LOCATE_DEVNODE_NORMAL = 0x00000000 // just to be crystal clear

// // Helper: get DEVINST from Instance ID string
// func getDevInstFromID(instanceID string) (uintptr, error) {
// 	logf("Attempting to locate devnode for ID: %q", instanceID)
// 	idPtr, err := windows.UTF16PtrFromString(instanceID)
// 	if err != nil {
// 		return 0, fmt.Errorf("UTF16 conversion failed: %w", err)
// 	}

// 	var devInst uintptr
// 	r, _, err := procCM_Locate_DevNodeW.Call(
// 		uintptr(unsafe.Pointer(&devInst)),
// 		uintptr(unsafe.Pointer(idPtr)),
// 		CM_LOCATE_DEVNODE_NORMAL,
// 	)
// 	if r != CR_SUCCESS {
// 		//return 0, fmt.Errorf("CM_Locate_DevNodeW failed: %d", r)
// 		var msg string
// 		switch r {
// 		case CR_INVALID_FLAG:
// 			msg = "CR_INVALID_FLAG (0xD) - likely invalid instance ID format or non-existent device"
// 		case CR_NO_SUCH_DEVNODE:
// 			msg = "CR_NO_SUCH_DEVNODE (0x19) - device does not exist"
// 		case CR_ACCESS_DENIED:
// 			msg = "CR_ACCESS_DENIED (0x21) - run as administrator?"
// 		case CR_NEED_RESTART:
// 			msg = "CR_NEED_RESTART (0x33) - "
// 		default:
// 			msg = fmt.Sprintf("unknown CR_ code 0x%X", r)
// 		}
// 		return 0, fmt.Errorf("CM_Locate_DevNodeW failed: %s (ret=%d, winerr=%v)", msg, r, err)
// 	}

// 	logf("Located devnode: 0x%X", devInst)
// 	return devInst, nil
// }

const WH_KEYBOARD_LL = 13

var (
	hookThreadId uint32
	kbdHook      windows.Handle
)

type KBDLLHOOKSTRUCT struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

// func installKeyboardHook() error {
// 	kbdProc := windows.NewCallback(keyboardProc)
// 	h, _, err := procSetWindowsHookEx.Call(
// 		WH_KEYBOARD_LL,
// 		uintptr(kbdProc),
// 		0, // hMod = 0 for low-level
// 		0, // dwThreadId = 0 = global
// 	)
// 	if h == 0 {
// 		return fmt.Errorf("SetWindowsHookEx(WH_KEYBOARD_LL) failed: %v", err)
// 	}
// 	kbdHook = windows.Handle(h)
// 	return nil
// }

func hookWorker() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hookThreadId = windows.GetCurrentThreadId()

	kbdProc := windows.NewCallback(keyboardProc)
	// Install the keyboard hook
	h, _, err := procSetWindowsHookEx.Call(
		WH_KEYBOARD_LL,
		uintptr(kbdProc),
		0, 0,
	)
	if h == 0 {
		logf("Failed to install keyboard hook: %v", err)
		return
	}
	kbdHook = windows.Handle(h)

	// Defer unhook (runs even on panic)
	defer func() {
		procUnhookWindowsHookEx.Call(uintptr(kbdHook))
		kbdHook = 0
		logf("Keyboard hook unhooked")
	}()

	// Private GetMessage loop — this is what makes the hook fire!
	var msg MSG
	for {
		ret, _, _ := procGetMessage.Call(
			uintptr(unsafe.Pointer(&msg)), 0, 0, 0,
		)
		const minus1 = ^uintptr(0)
		if int32(ret) <= 0 {
			logf("Hook worker thread received WM_QUIT(==0) or error(==%d) ret=%d, exiting and unhooking...", minus1, ret)
			break // WM_QUIT
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}

	logf("Hook worker done.")
}

const WM_QUIT = 0x0012

func deinit() {
	if hookThreadId != 0 {
		// Send WM_QUIT (0x0012) directly to the hook thread's message queue
		procPostThreadMessage.Call(uintptr(hookThreadId), WM_QUIT, 0, 0)
	}
	// rest of your cleanup...

	logf("Execution finished.")
}

type MSG struct {
	HWnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type POINT struct {
	X, Y int32
}

const (
	WM_KEYDOWN    = 0x0100
	WM_KEYUP      = 0x0101
	WM_SYSKEYDOWN = 0x0104
	WM_SYSKEYUP   = 0x0105

	HC_ACTION = 0
)

const (

	// Low-level keyboard hook flag
	LLKHF_INJECTED = 0x00000010
	// mouse:
	LLMHF_INJECTED = 0x00000001
)

func keyboardProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	kbd := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))
	logf("LL hook saw VK=0x%04X flags=0x%X wParam=0x%X, nCode=%d, injected:%d", kbd.VkCode, kbd.Flags, wParam, nCode, kbd.Flags&LLKHF_INJECTED)
	if nCode < 0 {
		//If nCode is less than zero, the hook procedure must pass the message to CallNextHookEx without further processing.
		goto next
		// ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
		// return ret
	}

	if nCode == 0 {
		kbd := (*KBDLLHOOKSTRUCT)(unsafe.Pointer(lParam))
		// no: Optional but recommended: ignore injected (synthetic) keys so we don't react to our own SendInput etc.
		//Many modern multimedia keyboards (Razer included, but also Logitech, Corsair, SteelSeries, etc.) internally generate media-key events using
		// the exact same injection mechanism that user-mode apps use. The firmware/microcontroller sends them to Windows as if they were synthetic
		// inputs — even though they come from physical key presses.
		// if kbd.Flags&LLKHF_INJECTED != 0 {
		// 	goto next
		// }

		vk := uint16(kbd.VkCode) // truncate to uint16 to match your map key type
		// // We only care about KeyDown of media keys
		// if wParam == WM_KEYDOWN || wParam == WM_SYSKEYDOWN {
		// vk := kbd.VkCode

		// Only process media/consumer keys we care about
		name, isMedia := mediaKeys[vk]
		if !isMedia {
			logf("key (down or up): VK=0x%02X", vk)
			goto next
		}
		logf("Media key (down or up): %s (VK=0x%02X)", name, vk)

		// ────────────────────────────────────────────────
		//  ↓  This is the migrated glitch detection logic  ↓
		// ────────────────────────────────────────────────

		/*
						    Mutex still required
						Even though most hooks run on the same thread (the thread that called GetMessage),
						Windows can re-enter the hook under rare conditions (especially with multiple keyboards or accessibility tools). Keep the mutex.Lock() / Unlock() pattern.
			      - Grok
		*/
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
			// Optional: log which key and how spammy it is
			logf("!!! GLITCH DETECTED: %s (VK=0x%02X) pressed %d times in %.1fs",
				name, vk, count, glitchWindow.Seconds())

			// Trigger reset (non-blocking from hook perspective)
			go func() { // ← important: do NOT block the hook callback!
				//logf("Not resetting, yet.")
				if true {
					if err := resetRazerKeyboardViaClassInstaller(); err != nil {
						logf("Reset failed: %v", err)
						// err = resetKeyboard()
						// if err != nil {
						// 	logf("powershell reset failed, err:%v", err)
						// }
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
		// }
	}

next:
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

func isMediaKey(vk uint32) bool {
	switch vk {
	case 0xB0, 0xB1, 0xB2, 0xB3, // next, prev, stop, play/pause
		0xAE, 0xAF: // vol up/down
		return true
	}
	return false
}

func logf(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	now := time.Now().Format("Mon Jan 2 15:04:05.000000000 MST 2006") // these values must be used exactly, they're like specific % placeholders.
	//now := time.Now().Format("2006-01-02T15:04:05.000Z07:00")
	finalMsg := fmt.Sprintf("[%s] %s\n", now, s)
	fmt.Print(finalMsg)
}

// ================== RESET USING DEVCON ==================

// resetRazerKeyboard finds the Razer device and restarts it using devcon64.exe (this works)
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
	h, _, err := procSetupDiGetClassDevs.Call(
		0,
		0,
		0,
		DIGCF_ALLCLASSES|DIGCF_PRESENT,
	)
	if h == 0 {
		return nil, err
	}
	defer procSetupDiDestroyDeviceInfoList.Call(h)

	var devices []Device

	for i := 0; ; i++ {
		var data SP_DEVINFO_DATA
		data.CbSize = uint32(unsafe.Sizeof(data))

		r1, _, _ := procSetupDiEnumDeviceInfo.Call(
			h,
			uintptr(i),
			uintptr(unsafe.Pointer(&data)),
		)
		if r1 == 0 {
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

func getInstanceID(h uintptr, data *SP_DEVINFO_DATA) (string, error) {
	buf := make([]uint16, 512)
	var required uint32
	r1, _, err := procSetupDiGetDeviceInstanceId.Call(
		h,
		uintptr(unsafe.Pointer(data)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&required)),
	)
	if r1 == 0 {
		return "", err
	}
	return windows.UTF16ToString(buf[:]), nil
}

func getDeviceDesc(h uintptr, data *SP_DEVINFO_DATA) (string, error) {
	buf := make([]byte, 512)
	var regType uint32
	var required uint32
	r1, _, err := procSetupDiGetDeviceRegistryProperty.Call(
		h,
		uintptr(unsafe.Pointer(data)),
		SPDRP_DEVICEDESC,
		uintptr(unsafe.Pointer(&regType)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&required)),
	)
	if r1 == 0 {
		return "", err
	}
	return windows.UTF16ToString((*[256]uint16)(unsafe.Pointer(&buf[0]))[:]), nil
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
	h, _, err := procSetupDiGetClassDevs.Call(
		0,
		0,
		0,
		DIGCF_ALLCLASSES|DIGCF_PRESENT,
	)
	if h == 0 {
		return fmt.Errorf("SetupDiGetClassDevs failed, ret=%d, err=%v", h, err)
	}
	defer procSetupDiDestroyDeviceInfoList.Call(h)

	for i := 0; ; i++ {
		var data SP_DEVINFO_DATA
		data.CbSize = uint32(unsafe.Sizeof(data))

		r1, _, _ := procSetupDiEnumDeviceInfo.Call(
			h,
			uintptr(i),
			uintptr(unsafe.Pointer(&data)),
		)
		if r1 == 0 {
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

func doPropertyChange(h uintptr, devInfo *SP_DEVINFO_DATA) error {
	// Prepare property change params
	var pcp SP_PROPCHANGE_PARAMS
	pcp.ClassInstallHeader.CbSize = uint32(unsafe.Sizeof(pcp.ClassInstallHeader))
	pcp.ClassInstallHeader.InstallFunction = DIF_PROPERTYCHANGE
	pcp.StateChange = DICS_PROPCHANGE // This is what devcon uses for "restart"
	pcp.Scope = DICS_FLAG_GLOBAL

	// Set the class install params
	r1, _, err := procSetupDiSetClassInstallParams.Call(
		h,
		uintptr(unsafe.Pointer(devInfo)),
		uintptr(unsafe.Pointer(&pcp.ClassInstallHeader)),
		uintptr(unsafe.Sizeof(pcp)),
	)
	if r1 == 0 {
		return fmt.Errorf("SetupDiSetClassInstallParams failed: %v", err)
	}

	// Call the class installer to perform the reset
	r1, _, err = procSetupDiCallClassInstaller.Call(
		DIF_PROPERTYCHANGE,
		h,
		uintptr(unsafe.Pointer(devInfo)),
	)
	// if r1 == 0 {
	// 	return fmt.Errorf("SetupDiCallClassInstaller (DIF_PROPERTYCHANGE) failed: %w", err)
	// }
	if r1 == 0 {
		who := "SetupDiCallClassInstaller (DIF_PROPERTYCHANGE)"
		// Defensive: err can be nil in some syscall edge cases
		if err != nil {
			// if errno, ok := err.(windows.Errno); ok {
			// 	if errno == windows.ERROR_ACCESS_DENIED {
			if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
				return fmt.Errorf("%s failed: %w: administrative privileges required", who, err)
			}
			return fmt.Errorf("%s failed: %w", who, err)
			//}
			// Non-Errno error (rare, but possible with wrappers)
			//return fmt.Errorf("%s failed (non-errno): %v", who, err)
		}

		// Extremely defensive fallback: syscall failed but err == nil
		return fmt.Errorf("SetupDiCallClassInstaller failed: unknown error (r1=0, err=nil)")
	}

	logf("Class installer reset(via DIF_PROPERTYCHANGE / DICS_PROPCHANGE) issued successfully.")
	return nil
}

// Add these constants and structs if not already present
const (
	DIF_PROPERTYCHANGE = 0x00000012
	DICS_PROPCHANGE    = 0x00000003
	DICS_FLAG_GLOBAL   = 0x00000001
)

type SP_CLASSINSTALL_HEADER struct {
	CbSize          uint32
	InstallFunction uint32
}

type SP_PROPCHANGE_PARAMS struct {
	ClassInstallHeader SP_CLASSINSTALL_HEADER
	StateChange        uint32
	Scope              uint32
	HwProfile          uint32
}
