package scm

import (
	"errors"
	"regexp"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	SERVICE_WIN32_MASK = windows.SERVICE_WIN32_OWN_PROCESS | windows.SERVICE_WIN32_SHARE_PROCESS
	
	SERVICE_CONFIG_DELAYED_AUTO_START_INFO = 3
	SERVICE_CONFIG_TRIGGER_INFO            = 8
)

var (
	// Regex matches base service name and a 5-8 char hex LUID suffix (e.g. _a1b2c).
	perUserSuffixRegex = regexp.MustCompile(`^(.+)_([0-9a-fA-F]{5,8})$`)
	
	modadvapi32             = windows.NewLazySystemDLL("advapi32.dll")
	procQueryServiceConfig2 = modadvapi32.NewProc("QueryServiceConfig2W")
	procChangeServiceConfig2 = modadvapi32.NewProc("ChangeServiceConfig2W")
)

type ServiceDetails struct {
	ServiceName   string
	DisplayName   string
	StartType     string
	DelayedStart  bool
	TriggerStart  bool
	IsPerUser     bool
	LiveStatus    uint32 // e.g. windows.SERVICE_RUNNING
}

type SERVICE_DELAYED_AUTO_START_INFO struct {
	IsDelayed uint32 // BOOL flag: 0 or 1
}

// NormalizePerUser determines if a service is a dynamically spawned user instance,
// returning the base template name and true if it is.
func NormalizePerUser(serviceName string) (string, bool) {
	matches := perUserSuffixRegex.FindStringSubmatch(serviceName)
	if len(matches) != 3 {
		return serviceName, false
	}
	baseName := matches[1]

	// Validate against the registry base template. 
	// If the template lacks UserServiceFlags, it might just be a coincidentally named standard service.
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\`+baseName, registry.QUERY_VALUE)
	if err != nil {
		return serviceName, false
	}
	defer k.Close()

	_, _, err = k.GetIntegerValue("UserServiceFlags")
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return serviceName, false
		}
	}

	return baseName, true
}

// EnumerateWin32Services fetches all user-mode services.
func EnumerateWin32Services() (map[string]ServiceDetails, error) {
	// Connect to SCM. SC_MANAGER_ENUMERATE_SERVICE is minimum required to list.
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_ENUMERATE_SERVICE|windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil, err
	}
	defer windows.CloseServiceHandle(scm)

	var bytesNeeded, servicesReturned, resumeHandle uint32
	// First pass to determine required buffer size.
	// Returns ERROR_MORE_DATA, but populates bytesNeeded.
	_ = windows.EnumServicesStatusEx(
		scm,
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
		return nil, errors.New("failed to retrieve required buffer size for SCM enumeration")
	}

	buf := make([]byte, bytesNeeded)
	err = windows.EnumServicesStatusEx(
		scm,
		windows.SC_ENUM_PROCESS_INFO,
		SERVICE_WIN32_MASK,
		windows.SERVICE_STATE_ALL,
		&buf[0],
		bytesNeeded,
		&bytesNeeded,
		&servicesReturned,
		&resumeHandle,
		nil,
	)
	
	if err != nil {
		return nil, err
	}

	result := make(map[string]ServiceDetails, servicesReturned)
	
	// Array of ENUM_SERVICE_STATUS_PROCESS structures.
	iter := uintptr(unsafe.Pointer(&buf[0]))
	structSize := unsafe.Sizeof(windows.ENUM_SERVICE_STATUS_PROCESS{})

	for i := uint32(0); i < servicesReturned; i++ {
		status := (*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(iter))
		rawSvcName := windows.UTF16PtrToString(status.ServiceName)
		displayName := windows.UTF16PtrToString(status.DisplayName)

		baseName, isPerUser := NormalizePerUser(rawSvcName)

		// Key preservation: Always map and store internally as lower-case for deterministic sorting 
		// and case-insensitive comparison, while preserving original canonical name in payload.
		lowerKey := strings.ToLower(baseName)

		details := ServiceDetails{
			ServiceName: baseName, // Store base template name
			DisplayName: displayName,
			IsPerUser:   isPerUser,
			LiveStatus:  status.ServiceStatusProcess.CurrentState,
		}

		enrichServiceConfig(scm, rawSvcName, &details)
		result[lowerKey] = details

		iter += structSize
	}

	return result, nil
}

// enrichServiceConfig opens the specific service and queries standard + delayed + trigger properties.
func enrichServiceConfig(scm windows.Handle, rawName string, details *ServiceDetails) {
	// SERVICE_QUERY_CONFIG is required for QueryServiceConfig(2).
	hSvc, err := windows.OpenService(scm, windows.StringToUTF16Ptr(rawName), windows.SERVICE_QUERY_CONFIG)
	if err != nil {
		// Often ERROR_ACCESS_DENIED (0x5) for protected drivers/services even if elevated.
		return
	}
	defer windows.CloseServiceHandle(hSvc)

	var bytesNeeded uint32
	_ = windows.QueryServiceConfig(hSvc, nil, 0, &bytesNeeded)
	if bytesNeeded > 0 {
		buf := make([]byte, bytesNeeded)
		if err := windows.QueryServiceConfig(hSvc, (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buf[0])), bytesNeeded, &bytesNeeded); err == nil {
			config := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buf[0]))
			details.StartType = mapWin32StartType(config.StartType)
		}
	}

	// Query Delayed Auto-Start
	_ = queryServiceConfig2(hSvc, SERVICE_CONFIG_DELAYED_AUTO_START_INFO, nil, 0, &bytesNeeded)
	if bytesNeeded > 0 {
		buf2 := make([]byte, bytesNeeded)
		if err := queryServiceConfig2(hSvc, SERVICE_CONFIG_DELAYED_AUTO_START_INFO, &buf2[0], bytesNeeded, &bytesNeeded); err == nil {
			delayedInfo := (*SERVICE_DELAYED_AUTO_START_INFO)(unsafe.Pointer(&buf2[0]))
			if delayedInfo.IsDelayed != 0 {
				details.DelayedStart = true
				if details.StartType == "Automatic" {
					details.StartType = "AutomaticDelayed"
				}
			}
		}
	}

	// Query Trigger Start 
	_ = queryServiceConfig2(hSvc, SERVICE_CONFIG_TRIGGER_INFO, nil, 0, &bytesNeeded)
	if bytesNeeded > 0 {
		buf3 := make([]byte, bytesNeeded)
		if err := queryServiceConfig2(hSvc, SERVICE_CONFIG_TRIGGER_INFO, &buf3[0], bytesNeeded, &bytesNeeded); err == nil {
			// Memory structure holds a pointer to trigger array.
			// Just having valid triggers returned indicates TriggerStart = true.
			// Further parsing requires mapping SERVICE_TRIGGER_INFO struct.
			details.TriggerStart = true
		}
	}
}

// queryServiceConfig2 wrappers the SyscallN for QueryServiceConfig2W
func queryServiceConfig2(hService windows.Handle, infoLevel uint32, buffer *byte, cbBufSize uint32, pcbBytesNeeded *uint32) error {
	ret, _, err := procQueryServiceConfig2.Call(
		uintptr(hService),
		uintptr(infoLevel),
		uintptr(unsafe.Pointer(buffer)),
		uintptr(cbBufSize),
		uintptr(unsafe.Pointer(pcbBytesNeeded)),
	)
	if ret == 0 {
		// err is atomically populated by the runtime from GetLastError
		return err
	}
	return nil
}

func mapWin32StartType(dwStartType uint32) string {
	switch dwStartType {
	case windows.SERVICE_DISABLED:
		return "Disabled"
	case windows.SERVICE_DEMAND_START:
		return "Manual"
	case windows.SERVICE_AUTO_START:
		return "Automatic"
	case windows.SERVICE_BOOT_START:
		return "Boot"
	case windows.SERVICE_SYSTEM_START:
		return "System"
	default:
		return "Unknown"
	}
}