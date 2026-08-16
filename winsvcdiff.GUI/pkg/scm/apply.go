package scm

import (
	"unsafe"
	"golang.org/x/sys/windows"
)

// SetServiceStartup Configures dwStartType and explicitly syncs the delayed flag.
func SetServiceStartup(scmHandle windows.Handle, serviceName string, targetState string) error {
	// Require SERVICE_CHANGE_CONFIG rights
	hSvc, err := windows.OpenService(scmHandle, windows.StringToUTF16Ptr(serviceName), windows.SERVICE_CHANGE_CONFIG)
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
		// Unsupported target via UI (e.g. Boot/System driver overrides not allowed)
		return windows.ERROR_INVALID_PARAMETER
	}

	// 1. Mutate base dwStartType. 
	// We pass SERVICE_NO_CHANGE (~0) for unchanged parameters.
	err = windows.ChangeServiceConfig(
		hSvc,
		windows.SERVICE_NO_CHANGE, 
		dwStartType,
		windows.SERVICE_NO_CHANGE,
		nil, nil, nil, nil, nil, nil, nil,
	)
	if err != nil {
		return err
	}

	// 2. Explicitly sync SERVICE_CONFIG_DELAYED_AUTO_START_INFO.
	// We MUST pass FALSE when switching away from Automatic (Delayed) to clear registry flag.
	delayedInfo := SERVICE_DELAYED_AUTO_START_INFO{
		IsDelayed: 0,
	}
	if isDelayed {
		delayedInfo.IsDelayed = 1
	}

	ret, _, callErr := procChangeServiceConfig2.Call(
		uintptr(hSvc),
		uintptr(SERVICE_CONFIG_DELAYED_AUTO_START_INFO),
		uintptr(unsafe.Pointer(&delayedInfo)),
	)
	
	if ret == 0 {
		return callErr
	}

	return nil
}