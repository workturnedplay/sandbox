//go:build windows && amd64

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

package scm

import (
	"fmt"
	"regexp"
	// "strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	modadvapi32               = windows.NewLazySystemDLL("advapi32.dll")
	procQueryServiceConfig2W  = modadvapi32.NewProc("QueryServiceConfig2W")
	procChangeServiceConfig2W = modadvapi32.NewProc("ChangeServiceConfig2W")

	perUserRegex = regexp.MustCompile(`^(.+)_([0-9a-fA-F]{5,8})$`)
)

// EnumerateWin32Services queries live SCM, normalizes per-user service keys, and reads config flags.
func EnumerateWin32Services() (map[string]ServiceInfo, error) {
	scmHandle, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT|windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return nil, fmt.Errorf("OpenSCManager failed: %w", err)
	}
	defer windows.CloseServiceHandle(scmHandle)

	var bytesNeeded, servicesReturned, resumeHandle uint32

	// First invocation fetches required buffer size for ENUM_SERVICE_STATUS_PROCESS array
	_ = windows.EnumServicesStatusEx(
		scmHandle,
		windows.SC_ENUM_PROCESS_INFO,
		SERVICE_WIN32_MASK,
		windows.SERVICE_STATE_ALL,
		nil,
		0,
		&bytesNeeded,
		&servicesReturned,
		&resumeHandle,
		nil,
	)

	if bytesNeeded == 0 {
		return nil, fmt.Errorf("failed to retrieve SCM enumeration buffer requirement")
	}

	buffer := make([]byte, bytesNeeded)
	err = windows.EnumServicesStatusEx(
		scmHandle,
		windows.SC_ENUM_PROCESS_INFO,
		SERVICE_WIN32_MASK,
		windows.SERVICE_STATE_ALL,
		&buffer[0],
		bytesNeeded,
		&bytesNeeded,
		&servicesReturned,
		&resumeHandle,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("EnumServicesStatusEx failed: %w", err)
	}

	results := make(map[string]ServiceInfo, servicesReturned)
	entrySize := unsafe.Sizeof(windows.ENUM_SERVICE_STATUS_PROCESS{})

	for i := uint32(0); i < servicesReturned; i++ {
		entry := (*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buffer[uintptr(i)*entrySize]))
		rawName := windows.UTF16PtrToString(entry.ServiceName)
		displayName := windows.UTF16PtrToString(entry.DisplayName)

		baseName, isPerUser := normalizeServiceName(rawName)
		liveStatus := mapServiceState(entry.ServiceStatusProcess.CurrentState)

		// Inspect target service config via OpenService
		startType, delayed, trigger := queryServiceConfigDetails(scmHandle, rawName)

		info := ServiceInfo{
			Name:             baseName,
			LiveInstanceName: rawName,
			DisplayName:      displayName,
			StartType:        startType,
			DelayedStart:     delayed,
			TriggerStart:     trigger,
			IsPerUser:        isPerUser,
			LiveStatus:       liveStatus,
		}

		results[baseName] = info
	}

	return results, nil
}

// normalizeServiceName strips per-user dynamic hex suffixes and validates base registry templates.
func normalizeServiceName(rawName string) (string, bool) {
	// 1. Inspect registry key for UserServiceFlags DWORD presence
	keyPath := `SYSTEM\CurrentControlSet\Services\` + rawName
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, registry.QUERY_VALUE)
	if err == nil {
		flags, _, regErr := k.GetIntegerValue("UserServiceFlags")
		_ = k.Close()
		if regErr == nil && flags != 0 {
			if matches := perUserRegex.FindStringSubmatch(rawName); len(matches) == 3 {
				return matches[1], true
			}
			return rawName, true
		}
	}

	// 2. Pattern match name structure against _XXXXX hex suffix
	if matches := perUserRegex.FindStringSubmatch(rawName); len(matches) == 3 {
		baseTemplate := matches[1]
		templateKeyPath := `SYSTEM\CurrentControlSet\Services\` + baseTemplate
		tk, err := registry.OpenKey(registry.LOCAL_MACHINE, templateKeyPath, registry.QUERY_VALUE)
		if err == nil {
			_ = tk.Close()
			return baseTemplate, true
		}
	}

	return rawName, false
}

func queryServiceConfigDetails(scmHandle windows.Handle, serviceName string) (string, bool, bool) {
	sNamePtr, err := windows.UTF16PtrFromString(serviceName)
	if err != nil {
		return "Unknown", false, false
	}

	hSvc, err := windows.OpenService(scmHandle, sNamePtr, windows.SERVICE_QUERY_CONFIG)
	if err != nil {
		return "Unknown", false, false
	}
	defer windows.CloseServiceHandle(hSvc)

	var bytesNeeded uint32
	_ = windows.QueryServiceConfig(hSvc, nil, 0, &bytesNeeded)
	if bytesNeeded == 0 {
		return "Unknown", false, false
	}

	cfgBuf := make([]byte, bytesNeeded)
	qsc := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&cfgBuf[0]))
	err = windows.QueryServiceConfig(hSvc, qsc, bytesNeeded, &bytesNeeded)
	if err != nil {
		return "Unknown", false, false
	}

	delayed := false
	var delayedInfo SERVICE_DELAYED_AUTO_START_INFO
	var cbNeeded uint32
	r1, _, _ := procQueryServiceConfig2W.Call(
		uintptr(hSvc),
		uintptr(SERVICE_CONFIG_DELAYED_AUTO_START_INFO),
		uintptr(unsafe.Pointer(&delayedInfo)),
		uintptr(unsafe.Sizeof(delayedInfo)),
		uintptr(unsafe.Pointer(&cbNeeded)),
	)
	if r1 != 0 {
		delayed = delayedInfo.FDelayedAutostart != 0
	}

	trigger := false
	var triggerInfo SERVICE_TRIGGER_INFO
	r1, _, _ = procQueryServiceConfig2W.Call(
		uintptr(hSvc),
		uintptr(SERVICE_CONFIG_TRIGGER_SET),
		uintptr(unsafe.Pointer(&triggerInfo)),
		uintptr(unsafe.Sizeof(triggerInfo)),
		uintptr(unsafe.Pointer(&cbNeeded)),
	)
	if r1 != 0 && triggerInfo.CTriggers > 0 {
		trigger = true
	}

	startTypeStr := mapWin32StartType(qsc.StartType, delayed)
	return startTypeStr, delayed, trigger
}

func mapWin32StartType(dwStartType uint32, delayed bool) string {
	switch dwStartType {
	case windows.SERVICE_DISABLED:
		return "Disabled"
	case windows.SERVICE_DEMAND_START:
		return "Manual"
	case windows.SERVICE_AUTO_START:
		if delayed {
			return "AutomaticDelayed"
		}
		return "Automatic"
	case windows.SERVICE_BOOT_START:
		return "Boot"
	case windows.SERVICE_SYSTEM_START:
		return "System"
	default:
		return "Unknown"
	}
}

func mapServiceState(state uint32) string {
	switch state {
	case windows.SERVICE_RUNNING:
		return "Running"
	case windows.SERVICE_STOPPED:
		return "Stopped"
	case windows.SERVICE_PAUSED:
		return "Paused"
	case windows.SERVICE_START_PENDING:
		return "Start Pending"
	case windows.SERVICE_STOP_PENDING:
		return "Stop Pending"
	default:
		return "Unknown"
	}
}

// SetServiceStartup mutates SCM state and explicitly clears/sets DelayedAutostart.
func SetServiceStartup(scmHandle windows.Handle, serviceName string, targetState string) error {
	sNamePtr, err := windows.UTF16PtrFromString(serviceName)
	if err != nil {
		return err
	}

	hSvc, err := windows.OpenService(scmHandle, sNamePtr, windows.SERVICE_CHANGE_CONFIG)
	if err != nil {
		return err
	}
	defer windows.CloseServiceHandle(hSvc)

	var dwStartType uint32
	var isDelayed bool

	switch targetState {
	case "AutomaticDelayed":
		dwStartType = windows.SERVICE_AUTO_START
		isDelayed = true
	case "Automatic":
		dwStartType = windows.SERVICE_AUTO_START
		isDelayed = false
	case "Manual":
		dwStartType = windows.SERVICE_DEMAND_START
		isDelayed = false
	case "Disabled":
		dwStartType = windows.SERVICE_DISABLED
		isDelayed = false
	default:
		return fmt.Errorf("unsupported target startup state: %s", targetState)
	}

	// 1. Change base Win32 startup type
	err = windows.ChangeServiceConfig(
		hSvc,
		windows.SERVICE_NO_CHANGE,
		dwStartType,
		windows.SERVICE_NO_CHANGE,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("ChangeServiceConfig failed: %w", err)
	}

	// 2. Explicitly update DelayedAutostart flag (Pass 0 when non-delayed to clear registry state)
	var delayedInfo SERVICE_DELAYED_AUTO_START_INFO
	if isDelayed {
		delayedInfo.FDelayedAutostart = 1
	} else {
		delayedInfo.FDelayedAutostart = 0
	}

	r1, _, lastErr := procChangeServiceConfig2W.Call(
		uintptr(hSvc),
		uintptr(SERVICE_CONFIG_DELAYED_AUTO_START_INFO),
		uintptr(unsafe.Pointer(&delayedInfo)),
	)
	if r1 == 0 {
		return fmt.Errorf("ChangeServiceConfig2W (DelayedAutostart) failed: %w", lastErr)
	}

	return nil
}

// MutateService performs targeted mutation, updating both dynamic instances and base template keys if per-user.
func MutateService(serviceName string, liveInstanceName string, isPerUser bool, targetState string) error {
	scmHandle, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return err
	}
	defer windows.CloseServiceHandle(scmHandle)

	// Mutate active live SCM instance
	targetInstance := liveInstanceName
	if targetInstance == "" {
		targetInstance = serviceName
	}

	err = SetServiceStartup(scmHandle, targetInstance, targetState)
	if err != nil && !isPerUser {
		return err
	}

	// If per-user service, also apply change to base template key in SCM
	if isPerUser && serviceName != targetInstance {
		_ = SetServiceStartup(scmHandle, serviceName, targetState)
	}

	return err
}
