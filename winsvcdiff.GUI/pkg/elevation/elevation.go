package elevation

import (
	"errors"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	// Cached at startup to ensure consistent re-execution paths,
	// avoiding dynamic compute issues during active process state changes.
	startupCwd string
)

func init() {
	var err error
	startupCwd, err = os.Getwd()
	if err != nil {
		// Fallback to executable directory if Getwd fails due to permissions
		execPath, _ := os.Executable()
		if execPath != "" {
			startupCwd = filepath.Dir(execPath)
		}
	}
}

// IsElevated queries the current process token for TokenElevation.
func IsElevated() (bool, error) {
	var token windows.Token
	// Open current process token with query rights.
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token)
	if err != nil {
		return false, err
	}
	defer token.Close()

	var isElevated uint32
	var retLen uint32
	// Retrieve TokenElevation structure (DWORD).
	err = windows.GetTokenInformation(
		token,
		windows.TokenElevation,
		(*byte)(unsafe.Pointer(&isElevated)),
		uint32(unsafe.Sizeof(isElevated)),
		&retLen,
	)
	if err != nil {
		return false, err
	}

	return isElevated != 0, nil
}

// RelaunchElevated attempts to silently re-launch the current executable via ShellExecuteW.
// Returns an error if the user denies the UAC prompt or execution fails.
func RelaunchElevated() error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	verb := windows.StringToUTF16Ptr("runas")
	exe := windows.StringToUTF16Ptr(execPath)
	cwd := windows.StringToUTF16Ptr(startupCwd)
	
	// Re-construct args, skipping the binary itself (os.Args[0]).
	var argsPtr *uint16
	if len(os.Args) > 1 {
		argsStr := windows.MakeCmdLine(os.Args[1:])
		argsPtr = windows.StringToUTF16Ptr(argsStr)
	}

	// ShellExecuteW directly calls into shell32.dll.
	// We use SW_SHOWNORMAL (1) to let the OS handle the target process UI mapping.
	err = windows.ShellExecute(
		0,
		verb,
		exe,
		argsPtr,
		cwd,
		windows.SW_SHOWNORMAL,
	)
	
	if err != nil {
		// 1223 = ERROR_CANCELLED (User refused UAC)
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return errors.New("user refused UAC elevation")
		}
		return err
	}

	return nil
}