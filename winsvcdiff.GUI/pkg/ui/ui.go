package ui

import (
	"fmt"
	"syscall"
	"unsafe"

	"github.com/gonutz/w32/v2"
	"golang.org/x/sys/windows"
)

const (
	className = "WinSvcDiffMainWindow"
	
	// Control IDs
	idBtnSave    = 1001
	idBtnLoad    = 1002
	idBtnRefresh = 1003
	idBtnApply   = 1004
	idTabCtrl    = 1005
	idListView   = 1006
	idComboBox   = 1007 // Floating inline editor
)

var (
	hMainWnd   w32.HWND
	hTab       w32.HWND
	hListView  w32.HWND
	hComboBox  w32.HWND
	hBtnSave   w32.HWND
	hBtnLoad   w32.HWND
	hBtnRef    w32.HWND
	hBtnApply  w32.HWND
	hFontDef   w32.HFONT

	// State tracking for the floating combobox
	editingRow int = -1
	editingCol int = -1
)

// Run initializes the Win32 message loop and renders the application frame.
func Run() error {
	hInstance := w32.GetModuleHandle("")

	// 1. Register Main Window Class
	var wc w32.WNDCLASSEX
	wc.Size = uint32(unsafe.Sizeof(wc))
	wc.Style = w32.CS_HREDRAW | w32.CS_VREDRAW
	wc.WndProc = syscall.NewCallback(wndProc)
	wc.Instance = hInstance
	wc.Cursor = w32.LoadCursor(0, w32.MakeIntResource(w32.IDC_ARROW))
	wc.Background = w32.COLOR_BTNFACE + 1
	wc.ClassName = syscall.StringToUTF16Ptr(className)

	if w32.RegisterClassEx(&wc) == 0 {
		return fmt.Errorf("RegisterClassEx failed: %v", windows.GetLastError())
	}

	// 2. Create Main Window
	hMainWnd = w32.CreateWindowEx(
		0,
		syscall.StringToUTF16Ptr(className),
		syscall.StringToUTF16Ptr("Win11 Service State Snapshot & Auditor"),
		w32.WS_OVERLAPPEDWINDOW|w32.WS_VISIBLE,
		w32.CW_USEDEFAULT, w32.CW_USEDEFAULT,
		950, 600,
		0, 0, hInstance, nil,
	)

	if hMainWnd == 0 {
		return fmt.Errorf("CreateWindowEx failed: %v", windows.GetLastError())
	}

	// 3. Message Pump
	var msg w32.MSG
	for w32.GetMessage(&msg, 0, 0, 0) != 0 {
		w32.TranslateMessage(&msg)
		w32.DispatchMessage(&msg)
	}

	// Cleanup GDI objects
	if hFontDef != 0 {
		w32.DeleteObject(w32.HGDIOBJ(hFontDef))
	}

	return nil
}

func wndProc(hwnd w32.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case w32.WM_CREATE:
		onCreate(hwnd)
		return 0

	case w32.WM_SIZE:
		onSize(hwnd, w32.LOWORD(uint32(lParam)), w32.HIWORD(uint32(lParam)))
		return 0

	case w32.WM_NOTIFY:
		return onNotify(hwnd, lParam)

	case w32.WM_COMMAND:
		return onCommand(hwnd, wParam, lParam)

	case w32.WM_DESTROY:
		w32.PostQuitMessage(0)
		return 0
	}
	return w32.DefWindowProc(hwnd, msg, wParam, lParam)
}

func onCreate(hwnd w32.HWND) {
	hInstance := w32.GetModuleHandle("")

	// Load default GUI font (Segoe UI on Win11)
	var logFont w32.LOGFONT
	w32.SystemParametersInfo(w32.SPI_GETICONTITLELOGFONT, uint32(unsafe.Sizeof(logFont)), unsafe.Pointer(&logFont), 0)
	hFontDef = w32.CreateFontIndirect(&logFont)

	// Action Buttons
	hBtnSave = createButton("Save Current State...", 10, 10, 150, 30, hwnd, idBtnSave, hInstance)
	hBtnLoad = createButton("Load State File...", 170, 10, 150, 30, hwnd, idBtnLoad, hInstance)
	hBtnRef = createButton("↻ Refresh System State", 330, 10, 180, 30, hwnd, idBtnRefresh, hInstance)

	// Tab Control
	hTab = w32.CreateWindowEx(
		0, syscall.StringToUTF16Ptr("SysTabControl32"), nil,
		w32.WS_CHILD|w32.WS_VISIBLE|w32.WS_CLIPSIBLINGS,
		10, 50, 910, 30,
		hwnd, w32.HMENU(idTabCtrl), hInstance, nil,
	)
	w32.SendMessage(hTab, w32.WM_SETFONT, uintptr(hFontDef), 1)

	insertTab(hTab, 0, "Tab 1: Mismatched")
	insertTab(hTab, 1, "Tab 2: Live Only")
	insertTab(hTab, 2, "Tab 3: File Only")
	insertTab(hTab, 3, "Tab 4: Matched")

	// List-View Control (LVS_REPORT for details grid)
	hListView = w32.CreateWindowEx(
		w32.WS_EX_CLIENTEDGE,
		syscall.StringToUTF16Ptr("SysListView32"), nil,
		w32.WS_CHILD|w32.WS_VISIBLE|w32.LVS_REPORT|w32.LVS_SINGLESEL|w32.LVS_SHOWSELALWAYS,
		10, 80, 910, 430,
		hwnd, w32.HMENU(idListView), hInstance, nil,
	)
	w32.SendMessage(hListView, w32.WM_SETFONT, uintptr(hFontDef), 1)

	// Set Gridlines and Checkboxes (LVS_EX_CHECKBOXES | LVS_EX_FULLROWSELECT | LVS_EX_GRIDLINES)
	exStyle := w32.LVS_EX_CHECKBOXES | w32.LVS_EX_FULLROWSELECT | w32.LVS_EX_GRIDLINES
	w32.SendMessage(hListView, w32.LVM_SETEXTENDEDLISTVIEWSTYLE, 0, uintptr(exStyle))

	insertListColumn(hListView, 0, "Service Name", 150)
	insertListColumn(hListView, 1, "Display Name", 250)
	insertListColumn(hListView, 2, "Live Status", 100)
	insertListColumn(hListView, 3, "Live Startup Type", 150)
	insertListColumn(hListView, 4, "Target (File)", 150)

	// Floating ComboBox (Invisible on creation)
	hComboBox = w32.CreateWindowEx(
		0,
		syscall.StringToUTF16Ptr("COMBOBOX"), nil,
		w32.WS_CHILD|w32.WS_BORDER|w32.CBS_DROPDOWNLIST|w32.WS_CLIPSIBLINGS,
		0, 0, 0, 0,
		hListView, w32.HMENU(idComboBox), hInstance, nil, // Note: Parent is the ListView, not MainWnd
	)
	w32.SendMessage(hComboBox, w32.WM_SETFONT, uintptr(hFontDef), 1)

	// Populate dropdown enum values
	addComboString(hComboBox, "Disabled")
	addComboString(hComboBox, "Manual")
	addComboString(hComboBox, "Automatic")
	addComboString(hComboBox, "AutomaticDelayed")

	// Apply Batch Button (Bottom Right)
	hBtnApply = createButton("Apply File State to Selected", 720, 520, 200, 30, hwnd, idBtnApply, hInstance)
}

func onSize(hwnd w32.HWND, width, height uint16) {
	// Dynamically resize children on frame scale
	if hTab != 0 {
		w32.MoveWindow(hTab, 10, 50, int(width)-20, 30, true)
	}
	if hListView != 0 {
		w32.MoveWindow(hListView, 10, 80, int(width)-20, int(height)-130, true)
	}
	if hBtnApply != 0 {
		w32.MoveWindow(hBtnApply, int(width)-220, int(height)-40, 200, 30, true)
	}
}

// onNotify handles events from the List-View, specifically NM_CLICK for positioning the inline ComboBox.
func onNotify(hwnd w32.HWND, lParam uintptr) uintptr {
	hdr := (*w32.NMHDR)(unsafe.Pointer(lParam))

	if hdr.IdFrom == idListView {
		// NM_CLICK inside the SysListView32
		if hdr.Code == w32.NM_CLICK {
			nmii := (*w32.NMITEMACTIVATE)(unsafe.Pointer(lParam))
			row := int(nmii.IItem)
			col := int(nmii.ISubItem)

			// If clicked row is valid and column is "Live Startup Type" (Index 3)
			if row != -1 && col == 3 {
				// Retrieve bounding box of the subitem
				var rect w32.RECT
				rect.Top = int32(col) // LVM_GETSUBITEMRECT requires the column index in 'Top'
				rect.Left = w32.LVIR_BOUNDS

				res := w32.SendMessage(hListView, w32.LVM_GETSUBITEMRECT, uintptr(row), uintptr(unsafe.Pointer(&rect)))
				if res != 0 {
					// Position and display the floating ComboBox exactly over the cell
					w32.MoveWindow(hComboBox, int(rect.Left), int(rect.Top), int(rect.Right-rect.Left), 100, true)
					
					// Update tracking state to apply mutations later
					editingRow = row
					editingCol = col

					// TODO: Extract current text from subitem to select the correct dropdown index
					// w32.SendMessage(hComboBox, w32.CB_SETCURSEL, uintptr(index), 0)

					w32.ShowWindow(hComboBox, w32.SW_SHOW)
					w32.SetFocus(hComboBox)
				}
			} else {
				// Clicked elsewhere, hide combobox
				w32.ShowWindow(hComboBox, w32.SW_HIDE)
			}
		}
	}
	return 0
}

// onCommand handles button presses and floating ComboBox selections.
func onCommand(hwnd w32.HWND, wParam, lParam uintptr) uintptr {
	controlID := w32.LOWORD(uint32(wParam))
	notificationCode := w32.HIWORD(uint32(wParam))

	switch controlID {
	case idBtnRefresh:
		// Trigger SCM Rescan
		// go triggerSCMRescan()
		return 0

	case idComboBox:
		// The inline combobox selection was finalized
		if notificationCode == w32.CBN_SELCHANGE {
			idx := w32.SendMessage(hComboBox, w32.CB_GETCURSEL, 0, 0)
			if idx != w32.CB_ERR && editingRow != -1 {
				// 1. Fetch text from combobox
				len := w32.SendMessage(hComboBox, w32.CB_GETLBTEXTLEN, uintptr(idx), 0)
				buf := make([]uint16, len+1)
				w32.SendMessage(hComboBox, w32.CB_GETLBTEXT, uintptr(idx), uintptr(unsafe.Pointer(&buf[0])))
				selectedState := syscall.UTF16ToString(buf)

				// 2. Fetch Service Name from column 0
				// Note: Use LVM_GETITEMTEXT to extract serviceName from column 0
				
				// 3. Fire immediate Win32 SCM mutation (pkg/scm)
				// err := scm.SetServiceStartup(scmHandle, serviceName, selectedState)
				
				// 4. Update List-View cell visually & Hide Box
				setListSubItemText(hListView, editingRow, editingCol, selectedState)
				w32.ShowWindow(hComboBox, w32.SW_HIDE)
			}
		} else if notificationCode == w32.CBN_KILLFOCUS {
			w32.ShowWindow(hComboBox, w32.SW_HIDE)
		}
		return 0
	}
	return 0
}

// --- Win32 UI Helpers ---

func createButton(text string, x, y, w, h int, parent w32.HWND, id int, hInst w32.HINSTANCE) w32.HWND {
	btn := w32.CreateWindowEx(
		0, syscall.StringToUTF16Ptr("BUTTON"), syscall.StringToUTF16Ptr(text),
		w32.WS_TABSTOP|w32.WS_VISIBLE|w32.WS_CHILD|w32.BS_PUSHBUTTON,
		x, y, w, h,
		parent, w32.HMENU(id), hInst, nil,
	)
	w32.SendMessage(btn, w32.WM_SETFONT, uintptr(hFontDef), 1)
	return btn
}

func insertTab(hTab w32.HWND, index int, text string) {
	var tci w32.TCITEM
	tci.Mask = w32.TCIF_TEXT
	utf16Text := syscall.StringToUTF16(text)
	tci.PszText = &utf16Text[0]
	w32.SendMessage(hTab, w32.TCM_INSERTITEM, uintptr(index), uintptr(unsafe.Pointer(&tci)))
}

func insertListColumn(hList w32.HWND, index int32, text string, width int32) {
	var lvc w32.LVCOLUMN
	lvc.Mask = w32.LVCF_FMT | w32.LVCF_WIDTH | w32.LVCF_TEXT | w32.LVCF_SUBITEM
	lvc.Fmt = w32.LVCFMT_LEFT
	lvc.Cx = width
	utf16Text := syscall.StringToUTF16(text)
	lvc.PszText = &utf16Text[0]
	lvc.ISubItem = index
	w32.SendMessage(hList, w32.LVM_INSERTCOLUMN, uintptr(index), uintptr(unsafe.Pointer(&lvc)))
}

func setListSubItemText(hList w32.HWND, row, col int, text string) {
	var lvi w32.LVITEM
	lvi.Mask = w32.LVIF_TEXT
	lvi.IItem = int32(row)
	lvi.ISubItem = int32(col)
	utf16Text := syscall.StringToUTF16(text)
	lvi.PszText = &utf16Text[0]
	w32.SendMessage(hList, w32.LVM_SETITEMTEXT, uintptr(row), uintptr(unsafe.Pointer(&lvi)))
}

func addComboString(hCombo w32.HWND, text string) {
	utf16Str := syscall.StringToUTF16Ptr(text)
	w32.SendMessage(hCombo, w32.CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Str)))
}