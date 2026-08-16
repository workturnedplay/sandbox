package ui

import (
	"syscall"
	"unsafe"

	"github.com/gonutz/w32/v2"
)

// showSaveFileDialog blocks the thread until the user selects a destination path or cancels.
func showSaveFileDialog(hwndOwner w32.HWND) string {
	var ofn w32.OPENFILENAME
	buf := make([]uint16, 260) // MAX_PATH

	ofn.LStructSize = uint32(unsafe.Sizeof(ofn))
	ofn.HwndOwner = hwndOwner
	// Filter structure: "Display Name\x00*.ext\x00" terminated by double \x00
	ofn.LpstrFilter = syscall.StringToUTF16Ptr("JSON State Files (*.json)\x00*.json\x00All Files (*.*)\x00*.*\x00\x00")
	ofn.LpstrFile = &buf[0]
	ofn.NMaxFile = uint32(len(buf))
	ofn.LpstrDefExt = syscall.StringToUTF16Ptr("json")
	// OFN_OVERWRITEPROMPT ensures the OS handles the "File exists, overwrite?" warning.
	ofn.Flags = w32.OFN_OVERWRITEPROMPT | w32.OFN_PATHMUSTEXIST

	if w32.GetSaveFileName(&ofn) {
		return syscall.UTF16ToString(buf)
	}
	return "" // User cancelled
}

// showOpenFileDialog blocks the thread until the user selects a source path or cancels.
func showOpenFileDialog(hwndOwner w32.HWND) string {
	var ofn w32.OPENFILENAME
	buf := make([]uint16, 260)

	ofn.LStructSize = uint32(unsafe.Sizeof(ofn))
	ofn.HwndOwner = hwndOwner
	ofn.LpstrFilter = syscall.StringToUTF16Ptr("JSON State Files (*.json)\x00*.json\x00All Files (*.*)\x00*.*\x00\x00")
	ofn.LpstrFile = &buf[0]
	ofn.NMaxFile = uint32(len(buf))
	ofn.Flags = w32.OFN_FILEMUSTEXIST | w32.OFN_PATHMUSTEXIST

	if w32.GetOpenFileName(&ofn) {
		return syscall.UTF16ToString(buf)
	}
	return ""
}