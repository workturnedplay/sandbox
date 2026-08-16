// pkg/ui/ui_actions.go
package ui

import (
	"syscall"
	"unsafe"
	"winsvcdiff/pkg/scm"
	
	"github.com/gonutz/w32/v2"
	"golang.org/x/sys/windows"
)

// getListSubItemText queries SysListView32 for the UTF-16 string at a specific cell.
func getListSubItemText(hList w32.HWND, row, col int) string {
	// Allocate buffer capable of holding MAX_PATH + padding.
	buf := make([]uint16, 260)

	var lvi w32.LVITEM
	lvi.ISubItem = int32(col)
	lvi.CchTextMax = int32(len(buf))
	lvi.PszText = &buf[0]

	// SendMessage LVM_GETITEMTEXTW returns the number of characters copied.
	res := w32.SendMessage(hList, w32.LVM_GETITEMTEXT, uintptr(row), uintptr(unsafe.Pointer(&lvi)))
	if res > 0 {
		return syscall.UTF16ToString(buf[:res])
	}
	return ""
}

// refreshListView clears the current ListView and loads fresh SCM data.
// In a full application, this would pipe through the pkg/state DiffResult.
func triggerSCMRescan() {
	// LVM_DELETEALLITEMS immediately clears all rows.
	w32.SendMessage(hListView, w32.LVM_DELETEALLITEMS, 0, 0)

	services, err := scm.GetAllServices()
	if err != nil {
		showErrorBox(fmt.Sprintf("Failed to enumerate SCM: %v", err))
		return
	}

	// Disable redraw during bulk insertion for performance.
	w32.SendMessage(hListView, w32.WM_SETREDRAW, 0, 0)
	defer func() {
		w32.SendMessage(hListView, w32.WM_SETREDRAW, 1, 0)
		w32.InvalidateRect(hListView, nil, true)
	}()

	row := 0
	for _, svc := range services {
		insertListRow(hListView, row, svc.Name)
		setListSubItemText(hListView, row, 1, svc.DisplayName)
		setListSubItemText(hListView, row, 2, svc.Status)
		setListSubItemText(hListView, row, 3, svc.Startup)
		setListSubItemText(hListView, row, 4, svc.ExePath)
		row++
	}
}

// insertListRow creates a new row item at the specified index.
func insertListRow(hList w32.HWND, index int, text string) {
	var lvi w32.LVITEM
	lvi.Mask = w32.LVIF_TEXT
	lvi.IItem = int32(index)
	lvi.ISubItem = 0
	utf16Text := syscall.StringToUTF16(text)
	lvi.PszText = &utf16Text[0]
	w32.SendMessage(hList, w32.LVM_INSERTITEM, 0, uintptr(unsafe.Pointer(&lvi)))
}

// showErrorBox displays an OS-native modal error prompt.
func showErrorBox(msg string) {
	w32.MessageBox(hMainWnd, msg, "WinSvcDiff Error", w32.MB_ICONERROR|w32.MB_OK)
}

// Replacing the incomplete blocks in wndProc's onNotify/onCommand from previously:
func handleComboBoxSelection() {
	idx := w32.SendMessage(hComboBox, w32.CB_GETCURSEL, 0, 0)
	if idx != w32.CB_ERR && editingRow != -1 {
		// 1. Fetch text from the floating combobox
		textLen := w32.SendMessage(hComboBox, w32.CB_GETLBTEXTLEN, uintptr(idx), 0)
		if textLen > 0 {
			buf := make([]uint16, textLen+1)
			w32.SendMessage(hComboBox, w32.CB_GETLBTEXT, uintptr(idx), uintptr(unsafe.Pointer(&buf[0])))
			selectedState := syscall.UTF16ToString(buf)

			// 2. Fetch the target Service Name from Column 0 of the selected row
			serviceName := getListSubItemText(hListView, editingRow, 0)

			if serviceName != "" {
				// 3. Fire immediate Win32 SCM mutation
				err := scm.SetServiceStartup(serviceName, selectedState)
				if err != nil {
					// Fallback: the service change failed (e.g. Access Denied)
					// Discard UI mutation and alert the user.
					w32.ShowWindow(hComboBox, w32.SW_HIDE)
					showErrorBox(fmt.Sprintf("Access Denied or configuration failure on %s:\n%v", serviceName, err))
					return
				}

				// 4. Mutation successful, update local UI state to reflect reality
				setListSubItemText(hListView, editingRow, editingCol, selectedState)
			}
		}
		w32.ShowWindow(hComboBox, w32.SW_HIDE)
	}
}

// Hook logic for populating the ComboBox index dynamically when opened:
func prepareInlineEditor(row, col int, rect w32.RECT) {
	w32.MoveWindow(hComboBox, int(rect.Left), int(rect.Top), int(rect.Right-rect.Left), 100, true)
	
	editingRow = row
	editingCol = col

	// Pre-select the item in the combo box corresponding to the current state in the ListView cell.
	currentValue := getListSubItemText(hListView, row, col)
	if currentValue != "" {
		utf16Value := syscall.StringToUTF16Ptr(currentValue)
		idx := w32.SendMessage(hComboBox, w32.CB_FINDSTRINGEXACT, w32.WPARAM(^uint32(0)), uintptr(unsafe.Pointer(utf16Value)))
		if idx != w32.CB_ERR {
			w32.SendMessage(hComboBox, w32.CB_SETCURSEL, uintptr(idx), 0)
		} else {
			// If unknown or empty, clear selection.
			w32.SendMessage(hComboBox, w32.CB_SETCURSEL, w32.WPARAM(^uint32(0)), 0)
		}
	}

	w32.ShowWindow(hComboBox, w32.SW_SHOW)
	w32.SetFocus(hComboBox)
}