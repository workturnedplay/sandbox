package ui

import (
	"fmt"
	"unsafe"

	"winsvcdiff/pkg/scm"
	"winsvcdiff/pkg/state"

	"github.com/gonutz/w32/v2"
)

var (
	// currentDiff caches the last diff operation to allow fast tab-switching
	// without needing to re-parse the JSON or re-query the SCM.
	currentDiff state.DiffResult
)

// onNotify intercepts List-View clicks and Tab Control switches.
func onNotify(hwnd w32.HWND, lParam uintptr) uintptr {
	hdr := (*w32.NMHDR)(unsafe.Pointer(lParam))

	switch hdr.IdFrom {
	case idListView:
		if hdr.Code == w32.NM_CLICK {
			nmii := (*w32.NMITEMACTIVATE)(unsafe.Pointer(lParam))
			row, col := int(nmii.IItem), int(nmii.ISubItem)

			if row != -1 && col == 3 {
				var rect w32.RECT
				rect.Top = int32(col) // LVM_GETSUBITEMRECT relies on Top for the column index
				rect.Left = w32.LVIR_BOUNDS

				if w32.SendMessage(hListView, w32.LVM_GETSUBITEMRECT, uintptr(row), uintptr(unsafe.Pointer(&rect))) != 0 {
					prepareInlineEditor(row, col, rect) // From ui_actions.go
				}
			} else {
				w32.ShowWindow(hComboBox, w32.SW_HIDE)
			}
		}
	case idTabCtrl:
		if hdr.Code == w32.TCN_SELCHANGE {
			// Tab changed. Repopulate the ListView from the cached currentDiff.
			idx := w32.SendMessage(hTab, w32.TCM_GETCURSEL, 0, 0)
			renderDiffTab(int(idx))
		}
	}
	return 0
}

// onCommand handles all Button clicks and ComboBox selection events.
func onCommand(hwnd w32.HWND, wParam, lParam uintptr) uintptr {
	controlID := w32.LOWORD(uint32(wParam))
	notificationCode := w32.HIWORD(uint32(wParam))

	switch controlID {
	case idBtnRefresh:
		triggerSCMRescan() // From ui_actions.go

	case idBtnSave:
		path := showSaveFileDialog(hwnd)
		if path == "" {
			return 0
		}
		live, err := scm.GetAllServices()
		if err != nil {
			showErrorBox(fmt.Sprintf("Failed to read SCM: %v", err))
			return 0
		}
		if err := state.Save(path, live); err != nil {
			showErrorBox(fmt.Sprintf("Failed to save state: %v", err))
		} else {
			w32.MessageBox(hwnd, "State saved successfully.", "Success", w32.MB_OK|w32.MB_ICONINFORMATION)
		}

	case idBtnLoad:
		path := showOpenFileDialog(hwnd)
		if path == "" {
			return 0
		}
		fileState, err := state.Load(path)
		if err != nil {
			showErrorBox(fmt.Sprintf("Failed to load JSON: %v", err))
			return 0
		}
		
		liveState, err := scm.GetAllServices()
		if err != nil {
			showErrorBox(fmt.Sprintf("SCM Error: %v", err))
			return 0
		}

		// Compute diff and render Tab 0 (Mismatched) by default
		currentDiff = state.Compare(liveState, fileState)
		w32.SendMessage(hTab, w32.TCM_SETCURSEL, 0, 0)
		renderDiffTab(0)

	case idComboBox:
		if notificationCode == w32.CBN_SELCHANGE {
			handleComboBoxSelection() // From ui_actions.go
		} else if notificationCode == w32.CBN_KILLFOCUS {
			w32.ShowWindow(hComboBox, w32.SW_HIDE)
		}
	}
	return 0
}

// renderDiffTab clears the ListView and populates it based on the selected tab index.
func renderDiffTab(tabIndex int) {
	w32.SendMessage(hListView, w32.WM_SETREDRAW, 0, 0)
	defer func() {
		w32.SendMessage(hListView, w32.WM_SETREDRAW, 1, 0)
		w32.InvalidateRect(hListView, nil, true)
	}()

	w32.SendMessage(hListView, w32.LVM_DELETEALLITEMS, 0, 0)

	row := 0
	switch tabIndex {
	case 0: // Mismatched
		for _, svc := range currentDiff.Mismatched {
			insertListRow(hListView, row, svc.Name)
			setListSubItemText(hListView, row, 1, svc.DisplayName)
			setListSubItemText(hListView, row, 2, "Live: "+svc.LiveStartup)
			setListSubItemText(hListView, row, 3, "File: "+svc.FileStartup)
			setListSubItemText(hListView, row, 4, "MISMATCH")
			row++
		}
	case 1: // Live Only
		for _, svc := range currentDiff.LiveOnly {
			insertStandardSvcRow(row, svc)
			row++
		}
	case 2: // File Only
		for _, svc := range currentDiff.FileOnly {
			insertStandardSvcRow(row, svc)
			row++
		}
	case 3: // Matched
		for _, svc := range currentDiff.Matched {
			insertStandardSvcRow(row, svc)
			row++
		}
	}
}

func insertStandardSvcRow(row int, svc scm.ServiceState) {
	insertListRow(hListView, row, svc.Name)
	setListSubItemText(hListView, row, 1, svc.DisplayName)
	setListSubItemText(hListView, row, 2, svc.Status)
	setListSubItemText(hListView, row, 3, svc.Startup)
	setListSubItemText(hListView, row, 4, svc.ExePath)
}