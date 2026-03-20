package main

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	setupapi                     = windows.NewLazySystemDLL("setupapi.dll")
	procSetupDiOpenDeviceInfo     = setupapi.NewProc("SetupDiOpenDeviceInfoW")
	procSetupDiGetClassDevs       = setupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiDestroyDeviceInfoList = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
	procSetupDiCallClassInstaller = setupapi.NewProc("SetupDiCallClassInstaller")
	procSetupDiSetClassInstallParams = setupapi.NewProc("SetupDiSetClassInstallParamsW")
)

type SP_DEVINFO_DATA struct {
	CbSize    uint32
	ClassGuid windows.GUID
	DevInst   uint32
	Reserved  uintptr
}

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

// Constants
const (
	DICS_ENABLE        = 1
	DICS_DISABLE       = 2
	DIF_PROPERTYCHANGE = 0x12
	DICS_FLAG_GLOBAL   = 0x00000001
	DICS_FLAG_CONFIGSPECIFIC = 0x00000002
)

// RestartDevice restarts a device by instance ID
func RestartDevice(instanceID string) error {
	hDevInfo, _, err := procSetupDiGetClassDevs.Call(
		0,
		0,
		0,
		0x00000004|0x00000002, // DIGCF_ALLCLASSES | DIGCF_PRESENT
	)
	if hDevInfo == 0 {
		return fmt.Errorf("SetupDiGetClassDevs failed: %v", err)
	}
	defer procSetupDiDestroyDeviceInfoList.Call(hDevInfo)

	var devData SP_DEVINFO_DATA
	devData.CbSize = uint32(unsafe.Sizeof(devData))

	instPtr, err := syscall.UTF16PtrFromString(instanceID)
	if err != nil {
		return err
	}
	r1, _, e1 := procSetupDiOpenDeviceInfo.Call(
		hDevInfo,
		uintptr(unsafe.Pointer(instPtr)),
		0,
		0,
		uintptr(unsafe.Pointer(&devData)),
	)
	if r1 == 0 {
		return fmt.Errorf("SetupDiOpenDeviceInfo failed: %v", e1)
	}

	// doPropChange mirrors devcon's dual-scope enable and error handling
	doPropChange := func(state uint32) error {
		scopes := []uint32{DICS_FLAG_CONFIGSPECIFIC}
		if state == DICS_ENABLE {
			// For enable, try global first, then config-specific
			scopes = []uint32{DICS_FLAG_GLOBAL, DICS_FLAG_CONFIGSPECIFIC}
		}

		var lastErr error
		for _, scope := range scopes {
      
      procSetupDiSetClassInstallParams.Call(hDevInfo, uintptr(unsafe.Pointer(&devData)), 0, 0) // clear stale state?!
      
			params := SP_PROPCHANGE_PARAMS{
				ClassInstallHeader: SP_CLASSINSTALL_HEADER{
					CbSize:          uint32(unsafe.Sizeof(SP_CLASSINSTALL_HEADER{})),
					InstallFunction: DIF_PROPERTYCHANGE,
				},
				StateChange: state,
				Scope:       scope,
				HwProfile:   0,
			}

			r1, _, e1 := procSetupDiSetClassInstallParams.Call(
				hDevInfo,
				uintptr(unsafe.Pointer(&devData)),
				uintptr(unsafe.Pointer(&params)),
				uintptr(unsafe.Sizeof(params)),
			)
			if r1 == 0 {
				lastErr = fmt.Errorf("SetupDiSetClassInstallParams failed (scope=0x%x): %v", scope, e1)
				if state == DICS_ENABLE && scope == DICS_FLAG_GLOBAL {
					// ignore global-enable failure and try config-specific next
					continue
				}
				//return lastErr
			}

			r1, _, e1 = procSetupDiCallClassInstaller.Call(
				DIF_PROPERTYCHANGE,
				hDevInfo,
				uintptr(unsafe.Pointer(&devData)),
			)
			if r1 == 0 {
				lastErr = fmt.Errorf("SetupDiCallClassInstaller failed (scope=0x%x): %v", scope, e1)
				if state == DICS_ENABLE && scope == DICS_FLAG_GLOBAL {
					continue
				}
				//return lastErr
			}
		}//for
		return lastErr
	}

	// Disable → Enable sequence
err = doPropChange(DICS_DISABLE)
if err != nil {
    fmt.Println("warning: disable failed, continuing:", err)
}
time.Sleep(100 * time.Millisecond)
if err = doPropChange(DICS_ENABLE); err != nil {
    return fmt.Errorf("enable failed: %v", err)
}

	return nil
}

func main() {
	instanceID := `USB\VID_1532&PID_0109\5&1E7D8DB7&0&14`
	fmt.Println("Restarting", instanceID)
	if err := RestartDevice(instanceID); err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Println("Restart completed")
	}
}