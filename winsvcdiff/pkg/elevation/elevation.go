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

package elevation

import (
	// "fmt"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// IsElevated checks whether the current process token possesses Administrative privileges.
func IsElevated() bool {
	var token windows.Token
	// Open current process token with TOKEN_QUERY access
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token)
	if err != nil {
		return false
	}
	defer token.Close()

	var elevation uint32
	var returnLen uint32

	// Query token elevation state via GetTokenInformation syscall
	err = windows.GetTokenInformation(
		token,
		windows.TokenElevation,
		(*byte)(unsafe.Pointer(&elevation)),
		uint32(unsafe.Sizeof(elevation)),
		&returnLen,
	)
	if err != nil {
		return false //, fmt.Errorf("GetTokenInformation(TokenElevation) failed: %w", err)
	}

	isElevated := elevation != 0

	return isElevated
}

// RelaunchElevated attempts to spawn an elevated child process using the Win32 'runas' verb.
// If the user accepts the UAC prompt, the non-elevated parent process exits silently (code 0).
// If the user declines or execution fails (ERROR_CANCELLED = 1223), the parent exits with code 1.
func RelaunchElevated() {
	exePath, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}

	verbPtr, _ := windows.UTF16PtrFromString("runas")
	exePtr, _ := windows.UTF16PtrFromString(exePath)

	// Preserve command-line arguments when elevating
	var argsPtr *uint16
	if len(os.Args) > 1 {
		argsStr := strings.Join(os.Args[1:], " ")
		argsPtr, _ = windows.UTF16PtrFromString(argsStr)
	}

	cwd, err := os.Getwd()
	var cwdPtr *uint16
	if err == nil {
		cwdPtr, _ = windows.UTF16PtrFromString(cwd)
	}

	var showCmd int32 = windows.SW_SHOWNORMAL

	// ShellExecuteW triggers the Windows UAC consent UI prompt
	err = windows.ShellExecute(0, verbPtr, exePtr, argsPtr, cwdPtr, showCmd)
	if err != nil {
		// User declined elevation prompt or OS policy prevented execution
		os.Exit(1)
	}

	// Elevating process spawned successfully; terminate non-elevated parent cleanly
	os.Exit(0)
}
