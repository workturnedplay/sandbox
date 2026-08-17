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

package wtw

// This file is deliberately the only file in the project that touches Win32.
// The rest of the package uses the Win32-independent types from wtw.go.

import (
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	serviceWin32Mask = windows.SERVICE_WIN32_OWN_PROCESS | windows.SERVICE_WIN32_SHARE_PROCESS

	wmAppUIReady   = 0x8000 + 1
	wmAppRunQueued = 0x8000 + 2
	wmAppShutdown  = 0x8000 + 3
	wmCommand      = 0x0111
	wmDestroy      = 0x0002
	wmClose        = 0x0010
	wmSize         = 0x0005
	wmNotify       = 0x004E
	wmSetFont      = 0x0030
	wmNCDestroy    = 0x0082
	wmVScroll      = 0x0115
	wmHScroll      = 0x0114

	swShown = 5
	swHide  = 0

	wsOverlappedWindow = 0x00CF0000
	wsChild            = 0x40000000
	wsVisible          = 0x10000000
	wsBorder           = 0x00800000
	wsTabStop          = 0x00010000
	wsVScroll          = 0x00200000
	wsHScroll          = 0x00100000
	wsClipChildren     = 0x02000000
	wsExClientEdge     = 0x00000200

	lvsReport          = 0x0001
	lvsSingleSel       = 0x0004
	lvsShowSelAlways   = 0x0008
	lvsExFullRowSelect = 0x00000020
	lvsExGridLines     = 0x00000001
	lvsExCheckBoxes    = 0x00000004

	lvmFirst                    = 0x1000
	lvmSetExtendedListViewStyle = lvmFirst + 54
	lvmInsertColumnW            = lvmFirst + 97
	lvmDeleteAllItems           = lvmFirst + 9
	lvmInsertItemW              = lvmFirst + 77
	lvmSetItemW                 = lvmFirst + 76
	lvmGetNextItem              = lvmFirst + 12
	lvmGetItemCount             = lvmFirst + 4
	lvmGetItemTextW             = lvmFirst + 115
	lvmGetSubItemRect           = lvmFirst + 56
	lvmGetItemState             = lvmFirst + 44
	lvmSetItemState             = lvmFirst + 43

	lvifText     = 0x0001
	lvirBounds   = 0
	lvniSelected = 0x0002
	lvniFocused  = 0x0001
	lvnisAll     = 0xFFFF

	lvmItemStateChecked   = 0x2000
	lvmItemStateUnchecked = 0x1000

	tcmFirst       = 0x1300
	tcmInsertItemW = tcmFirst + 62
	tcmGetCurSel   = tcmFirst + 11
	tcmSetCurSel   = tcmFirst + 12

	nmDblClk       = -3
	lvnColumnClick = -108
	tcnSelChange   = -551

	cbAddString     = 0x0143
	cbSetCurSel     = 0x014E
	cbGetCurSel     = 0x0147
	cbGetLbTextLen  = 0x0149
	cbGetLbText     = 0x0148
	cbsDropdownList = 0x0003

	cmbSelChange = 1

	lvmSetItemWCode = lvmSetItemW

	tcsTabs = 0x0008

	progressBarMarquee = 0x08
	pbmSetMarquee      = 0x040A
	pbsMarquee         = progressBarMarquee

	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010

	ofnExplorer        = 0x00080000
	ofnFileMustExist   = 0x00001000
	ofnPathMustExist   = 0x00000800
	ofnOverwritePrompt = 0x00000002
	ofnEnableSizing    = 0x00800000

	idSaveLive = 1001
	idLoadFile = 1002
	idRefresh  = 1003
	idApply    = 1004
	idAppend   = 1005
	idTab      = 1100
	idListBase = 1200
	idBusy     = 1300
	idStatus   = 1301
	idTarget   = 1302
	idProgress = 1303
	idCombo    = 1400

	lvifState         = 0x0008
	lvifParam         = 0x0008
	lvifImage         = 0x0002
	lvsExDoubleBuffer = 0x00010000

	lparamNotify = 0

	colorWindow = 5
	idcArrow    = 32512

	commonControlTab      = 0x00000008
	commonControlListView = 0x00000001
	commonControlProgress = 0x00000020
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	comdlg32 = windows.NewLazySystemDLL("comdlg32.dll")
	comctl32 = windows.NewLazySystemDLL("comctl32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procDefWindowProcW       = user32.NewProc("DefWindowProcW")
	procRegisterClassExW     = user32.NewProc("RegisterClassExW")
	procCreateWindowExW      = user32.NewProc("CreateWindowExW")
	procDestroyWindow        = user32.NewProc("DestroyWindow")
	procShowWindow           = user32.NewProc("ShowWindow")
	procUpdateWindow         = user32.NewProc("UpdateWindow")
	procGetMessageW          = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessageW     = user32.NewProc("DispatchMessageW")
	procPostQuitMessage      = user32.NewProc("PostQuitMessage")
	procPostMessageW         = user32.NewProc("PostMessageW")
	procSendMessageW         = user32.NewProc("SendMessageW")
	procSetWindowTextW       = user32.NewProc("SetWindowTextW")
	procMoveWindow           = user32.NewProc("MoveWindow")
	procGetClientRect        = user32.NewProc("GetClientRect")
	procGetSubMenu           = user32.NewProc("GetSubMenu")
	procLoadCursorW          = user32.NewProc("LoadCursorW")
	procMessageBoxW          = user32.NewProc("MessageBoxW")
	procGetModuleHandleW     = kernel32.NewProc("GetModuleHandleW")
	procGetConsoleMode       = kernel32.NewProc("GetConsoleMode")
	procShellExecuteW        = shell32.NewProc("ShellExecuteW")
	procInitCommonControlsEx = comctl32.NewProc("InitCommonControlsEx")
	procGetOpenFileNameW     = comdlg32.NewProc("GetOpenFileNameW")
	procGetSaveFileNameW     = comdlg32.NewProc("GetSaveFileNameW")
	procCommDlgExtendedError = comdlg32.NewProc("CommDlgExtendedError")
	procIsUserAnAdmin        = shell32.NewProc("IsUserAnAdmin")
)

// --- Elevation -------------------------------------------------------------

func IsElevated() bool {
	r1, _, _ := procIsUserAnAdmin.Call()
	return r1 != 0
}

func RelaunchElevated(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	verb := windows.StringToUTF16Ptr("runas")
	file := windows.StringToUTF16Ptr(exe)
	params, err := quoteWindowsArgs(args)
	if err != nil {
		return err
	}
	paramsPtr := windows.StringToUTF16Ptr(params)
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	cwdPtr := windows.StringToUTF16Ptr(cwd)
	show := uintptr(swShown)
	ret, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(paramsPtr)),
		uintptr(unsafe.Pointer(cwdPtr)),
		show,
	)
	if ret <= 32 {
		return fmt.Errorf("ShellExecuteW runas failed (return %d)", ret)
	}
	return nil
}

func quoteWindowsArgs(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = quoteWindowsArg(arg)
	}
	return strings.Join(quoted, " "), nil
}

// quoteWindowsArg implements the CommandLineToArgvW-compatible quoting rules
// required for ShellExecuteW's lpParameters string.
func quoteWindowsArg(arg string) string {
	if arg != "" && !strings.ContainsAny(arg, " \t\"") {
		return arg
	}
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range arg {
		if r == '\\' {
			backslashes++
			continue
		}
		if r == '"' {
			b.WriteString(strings.Repeat("\\", backslashes*2+1))
			b.WriteRune(r)
			backslashes = 0
			continue
		}
		if backslashes > 0 {
			b.WriteString(strings.Repeat("\\", backslashes))
			backslashes = 0
		}
		b.WriteRune(r)
	}
	if backslashes > 0 {
		b.WriteString(strings.Repeat("\\", backslashes*2))
	}
	b.WriteByte('"')
	return b.String()
}

// --- SCM enumeration/configuration ----------------------------------------

func EnumerateLiveServices() (map[string]LiveService, error) {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_ENUMERATE_SERVICE|windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil, fmt.Errorf("OpenSCManager: %w", err)
	}
	defer windows.CloseServiceHandle(scm)

	raw, err := enumServices(scm)
	if err != nil {
		return nil, err
	}
	result := make([]LiveService, 0, len(raw))
	for _, r := range raw {
		cfg, err := queryServiceConfig(scm, r.Name)
		if err != nil {
			return nil, fmt.Errorf("query service %q configuration: %w", r.Name, err)
		}
		perUser, base := detectPerUserService(r.Name)
		cfg.IsPerUser = perUser
		normalizedName := r.Name
		if perUser && base != "" {
			normalizedName = base
		}
		result = append(result, LiveService{
			Name:          normalizedName,
			Config:        cfg,
			CurrentStatus: serviceStateName(r.CurrentState),
			Instances:     []string{r.Name},
		})
	}
	return NormalizeLiveServices(result), nil
}

type enumServiceRecord struct {
	Name         string
	DisplayName  string
	CurrentState uint32
}

func enumServices(scm windows.Handle) ([]enumServiceRecord, error) {
	const initialSize = uint32(64 * 1024)
	const maxSize = uint32(16 * 1024 * 1024)
	size := initialSize
	resume := uint32(0)
	all := make([]enumServiceRecord, 0, 256)

	for {
		buffer := make([]byte, size)
		var needed, returned uint32
		err := windows.EnumServicesStatusEx(
			scm,
			windows.SC_ENUM_PROCESS_INFO,
			serviceWin32Mask,
			windows.SERVICE_STATE_ALL,
			&buffer[0],
			uint32(len(buffer)),
			&needed,
			&returned,
			&resume,
			nil,
		)
		if err != nil && !errors.Is(err, windows.ERROR_MORE_DATA) {
			return nil, fmt.Errorf("EnumServicesStatusEx: %w", err)
		}
		entrySize := uint64(unsafe.Sizeof(windows.ENUM_SERVICE_STATUS_PROCESS{}))
		if uint64(returned)*entrySize > uint64(len(buffer)) {
			return nil, fmt.Errorf("EnumServicesStatusEx returned %d records exceeding buffer", returned)
		}
		if returned > 0 {
			records := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buffer[0])), int(returned))
			for _, rec := range records {
				if rec.ServiceName == nil || rec.DisplayName == nil {
					return nil, errors.New("EnumServicesStatusEx returned a null service/display name")
				}
				all = append(all, enumServiceRecord{
					Name:         strings.TrimSpace(windows.UTF16PtrToString(rec.ServiceName)),
					DisplayName:  windows.UTF16PtrToString(rec.DisplayName),
					CurrentState: rec.ServiceStatusProcess.CurrentState,
				})
			}
		}
		if err == nil {
			break
		}
		if size >= maxSize {
			if needed > 0 && needed <= maxSize {
				size = needed
			} else {
				return nil, fmt.Errorf("EnumServicesStatusEx requires more than %d bytes", maxSize)
			}
		} else {
			next := size * 2
			if needed > next {
				next = needed
			}
			if next > maxSize {
				next = maxSize
			}
			size = next
		}
	}
	sort.Slice(all, func(i, j int) bool { return strings.ToLower(all[i].Name) < strings.ToLower(all[j].Name) })
	return all, nil
}

func queryServiceConfig(scm windows.Handle, serviceName string) (ServiceConfig, error) {
	svc, err := openService(scm, serviceName, windows.SERVICE_QUERY_CONFIG|windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return ServiceConfig{}, err
	}
	defer windows.CloseServiceHandle(svc)

	size := uint32(4096)
	for attempt := 0; attempt < 8; attempt++ {
		buf := make([]byte, size)
		var needed uint32
		err := windows.QueryServiceConfig(svc, (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buf[0])), uint32(len(buf)), &needed)
		if err == nil {
			q := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buf[0]))
			display := windows.UTF16PtrToString(q.DisplayName)
			delayed, err2 := queryDelayedStart(svc)
			if err2 != nil {
				return ServiceConfig{}, fmt.Errorf("QueryServiceConfig2 delayed-start: %w", err2)
			}
			trigger, err3 := queryTriggerStart(svc)
			if err3 != nil {
				return ServiceConfig{}, fmt.Errorf("QueryServiceConfig2 trigger-info: %w", err3)
			}
			return ServiceConfig{
				DisplayName:  display,
				StartType:    canonicalStartType(q.StartType, delayed),
				DelayedStart: delayed,
				TriggerStart: trigger,
			}, nil
		}
		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
			return ServiceConfig{}, err
		}
		if needed <= size {
			if size > math.MaxUint32/2 {
				return ServiceConfig{}, errors.New("QueryServiceConfig buffer size overflow")
			}
			size *= 2
		} else {
			size = needed
		}
	}
	return ServiceConfig{}, errors.New("QueryServiceConfig exceeded retry limit")
}

func openService(scm windows.Handle, serviceName string, access uint32) (windows.Handle, error) {
	ptr, err := windows.UTF16PtrFromString(serviceName)
	if err != nil {
		return 0, err
	}
	return windows.OpenService(scm, ptr, access)
}

func queryDelayedStart(svc windows.Handle) (bool, error) {
	var info struct{ Delayed uint32 }
	var needed uint32
	err := windows.QueryServiceConfig2(svc, windows.SERVICE_CONFIG_DELAYED_AUTO_START_INFO, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), &needed)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_INVALID_LEVEL) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Delayed != 0, nil
}

func queryTriggerStart(svc windows.Handle) (bool, error) {
	size := uint32(512)
	for attempt := 0; attempt < 8; attempt++ {
		buf := make([]byte, size)
		var needed uint32
		err := windows.QueryServiceConfig2(svc, windows.SERVICE_CONFIG_TRIGGER_INFO, &buf[0], size, &needed)
		if err == nil {
			if len(buf) < int(unsafe.Sizeof(serviceTriggerInfo{})) {
				return false, errors.New("SERVICE_CONFIG_TRIGGER_INFO response is truncated")
			}
			info := (*serviceTriggerInfo)(unsafe.Pointer(&buf[0]))
			return info.TriggerCount > 0 && info.Triggers != 0, nil
		}
		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
			if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_INVALID_LEVEL) {
				return false, nil
			}
			return false, err
		}
		if needed <= size {
			size *= 2
		} else {
			size = needed
		}
	}
	return false, errors.New("QueryServiceConfig2 trigger-info exceeded retry limit")
}

type serviceTriggerInfo struct {
	TriggerCount uint32
	Triggers     uintptr
}

func canonicalStartType(startType uint32, delayed bool) string {
	switch startType {
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
		return fmt.Sprintf("Unknown(0x%X)", startType)
	}
}

func serviceStateName(state uint32) string {
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
		return fmt.Sprintf("State 0x%X", state)
	}
}

func detectPerUserService(serviceName string) (bool, string) {
	const basePath = `SYSTEM\CurrentControlSet\Services\`
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, basePath+serviceName, registry.QUERY_VALUE)
	if err == nil {
		flags, _, flagErr := key.GetIntegerValue("UserServiceFlags")
		_ = key.Close()
		if flagErr == nil && flags != 0 {
			if base, ok := IsPerUserCandidate(serviceName); ok {
				return true, base
			}
			return true, serviceName
		}
	}
	base, ok := IsPerUserCandidate(serviceName)
	if !ok {
		return false, ""
	}
	if base == "" {
		return false, ""
	}
	templateKey, err := registry.OpenKey(registry.LOCAL_MACHINE, basePath+base, registry.QUERY_VALUE)
	if err != nil {
		return false, ""
	}
	_ = templateKey.Close()
	return true, base
}

func SetServiceStartup(instanceNames []string, baseName string, isPerUser bool, target string) error {
	startType, delayed, err := targetStartType(target)
	if err != nil {
		return err
	}
	names := appendUnique(nil, instanceNames...)
	if isPerUser && baseName != "" {
		names = appendUnique(names, baseName)
	}
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return fmt.Errorf("OpenSCManager for mutation: %w", err)
	}
	defer windows.CloseServiceHandle(scm)

	var errs []string
	for _, name := range names {
		ptr, err := windows.UTF16PtrFromString(name)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		svc, err := windows.OpenService(scm, ptr, windows.SERVICE_CHANGE_CONFIG)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		err1 := windows.ChangeServiceConfig(svc, windows.SERVICE_NO_CHANGE, startType, windows.SERVICE_NO_CHANGE, nil, nil, nil, nil, nil, nil, nil)
		err2 := error(nil)
		if err1 == nil {
			delayedInfo := struct{ Delayed uint32 }{}
			if delayed {
				delayedInfo.Delayed = 1
			}
			err2 = windows.ChangeServiceConfig2(svc, windows.SERVICE_CONFIG_DELAYED_AUTO_START_INFO, (*byte)(unsafe.Pointer(&delayedInfo)))
		}
		closeErr := windows.CloseServiceHandle(svc)
		if err1 != nil {
			errs = append(errs, fmt.Sprintf("%s: ChangeServiceConfig: %v", name, err1))
			continue
		}
		if err2 != nil {
			errs = append(errs, fmt.Sprintf("%s: ChangeServiceConfig2(delayed): %v", name, err2))
		}
		if closeErr != nil {
			errs = append(errs, fmt.Sprintf("%s: CloseServiceHandle: %v", name, closeErr))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}

func targetStartType(target string) (uint32, bool, error) {
	switch target {
	case "AutomaticDelayed":
		return windows.SERVICE_AUTO_START, true, nil
	case "Automatic":
		return windows.SERVICE_AUTO_START, false, nil
	case "Manual":
		return windows.SERVICE_DEMAND_START, false, nil
	case "Disabled":
		return windows.SERVICE_DISABLED, false, nil
	case "Boot":
		return windows.SERVICE_BOOT_START, false, nil
	case "System":
		return windows.SERVICE_SYSTEM_START, false, nil
	default:
		return 0, false, fmt.Errorf("unsupported target startup type %q", target)
	}
}

func SystemInfoNow() SystemInfo {
	hostname, _ := os.Hostname()
	build := ""
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err == nil {
		if n, _, e := key.GetStringValue("CurrentBuildNumber"); e == nil {
			build = n
		}
		_ = key.Close()
	}
	if build != "" {
		build = "10.0." + build
	}
	return SystemInfo{Hostname: hostname, OSBuild: build}
}

// --- Native GUI abstraction -------------------------------------------------

// UIRow is intentionally Win32-independent so callers do not need windows.Handle.
type UIRow struct {
	Key         string
	DisplayName string
	Status      string
	LiveStartup string
	Target      string
	Checked     bool
	Editable    bool
}

type UIHandlers struct {
	OnReady          func(*UI)
	OnAction         func(action string, ui *UI)
	OnTabChanged     func(tab int, ui *UI)
	OnStartupChanged func(tab int, serviceName, target string, ui *UI)
	OnClose          func(*UI)
}

type UI struct {
	hwnd         windows.Handle
	tab          windows.Handle
	lists        [4]windows.Handle
	buttons      map[string]windows.Handle
	status       windows.Handle
	target       windows.Handle
	progress     windows.Handle
	rows         [4][]UIRow
	originalRows [4][]UIRow
	sortColumn   [4]int
	sortOrder    [4]int // 0 = unsorted, 1 = A-Z, 2 = Z-A
	callbacks    UIHandlers
	queue        chan func()
	combo        windows.Handle
	comboTab     int
	comboRow     int
	shuttingDown bool
	busy         bool
	mu           sync.Mutex
}

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	WndProc       uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         windows.Handle
	HCursor       windows.Handle
	HbrBackground windows.Handle
	MenuName      *uint16
	ClassName     *uint16
	HIconSm       windows.Handle
}

type msg struct {
	Hwnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type nmhdr struct {
	HwndFrom windows.Handle
	IDFrom   uintptr
	Code     int32
}

type nmitemActivate struct {
	Hdr       nmhdr
	IItem     int32
	ISubItem  int32
	UNewState uint32
	UOldState uint32
	UChanged  uint32
	PtAction  point
	LParam    uintptr
}

type lvColumn struct {
	Mask    uint32
	Fmt     int32
	CX      int32
	Text    *uint16
	TextMax int32
	SubItem int32
	Image   int32
	Order   int32
	Columns *uint32
}

type lvItem struct {
	Mask      uint32
	Item      int32
	SubItem   int32
	State     uint32
	StateMask uint32
	Text      *uint16
	TextMax   int32
	Image     int32
	Param     uintptr
	Indent    int32
	GroupID   int32
	Columns   uintptr
	NoAnimate uintptr
	StateIcon uintptr
	Group     uintptr
}

type tabItem struct {
	Mask      uint32
	State     uint32
	StateMask uint32
	Text      *uint16
	TextMax   int32
	Image     int32
	Param     uintptr
}

type openFileName struct {
	StructSize    uint32
	Owner         windows.Handle
	Instance      windows.Handle
	Filter        *uint16
	CustomFilter  *uint16
	MaxCustFilter uint32
	FilterIndex   uint32
	File          *uint16
	MaxFile       uint32
	FileTitle     *uint16
	MaxFileTitle  uint32
	InitialDir    *uint16
	Title         *uint16
	Flags         uint32
	FileOffset    uint16
	FileExtension uint16
	DefExt        *uint16
	CustData      uintptr
	Hook          uintptr
	TemplateName  *uint16
	ReservedPtr   uintptr
	ReservedInt   uint32
	FlagsEx       uint32
}

type initCommonControlsEx struct {
	Size uint32
	ICC  uint32
}

func RunUI(handlers UIHandlers) int {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ui := &UI{
		callbacks:  handlers,
		buttons:    make(map[string]windows.Handle),
		queue:      make(chan func(), 128),
		sortColumn: [4]int{-1, -1, -1, -1},
	}
	if err := ui.initialize(); err != nil {
		return 1
	}
	if handlers.OnReady != nil {
		handlers.OnReady(ui)
	}

	var m msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) == -1 {
			return 1
		}
		if ret == 0 {
			return 0
		}
		if m.Message == wmAppRunQueued {
			ui.drainQueue()
			continue
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (ui *UI) initialize() error {
	icc := initCommonControlsEx{Size: uint32(unsafe.Sizeof(initCommonControlsEx{})), ICC: commonControlTab | commonControlListView | commonControlProgress}
	if r, _, _ := procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc))); r == 0 {
		return errors.New("InitCommonControlsEx failed")
	}
	hinst, _, _ := procGetModuleHandleW.Call(0)
	if hinst == 0 {
		return errors.New("GetModuleHandleW failed")
	}
	className := windows.StringToUTF16Ptr("WinSvcDiffMainWindow")
	wc := wndClassEx{
		CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:       windows.NewCallback(ui.wndProc),
		HInstance:     windows.Handle(hinst),
		HCursor:       ui.loadCursor(),
		HbrBackground: windows.Handle(colorWindow + 1),
		ClassName:     className,
	}
	if r, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		// ERROR_CLASS_ALREADY_EXISTS is benign for a second UI instance in
		// the same process, but this application never creates two.
		if err := windows.GetLastError(); err != windows.ERROR_CLASS_ALREADY_EXISTS {
			return fmt.Errorf("RegisterClassExW: %w", err)
		}
	}
	title := windows.StringToUTF16Ptr("Win11 Service State Snapshot & Auditor")
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow|wsClipChildren,
		100, 100, 1280, 760,
		0, 0, uintptr(hinst), 0,
	)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW: %w", windows.GetLastError())
	}
	ui.hwnd = windows.Handle(hwnd)
	if err := ui.createControls(windows.Handle(hinst)); err != nil {
		return err
	}
	procShowWindow.Call(uintptr(ui.hwnd), swShown)
	procUpdateWindow.Call(uintptr(ui.hwnd))
	return nil
}

func (ui *UI) loadCursor() windows.Handle {
	h, _, _ := procLoadCursorW.Call(0, uintptr(idcArrow))
	return windows.Handle(h)
}

func (ui *UI) createControls(hinst windows.Handle) error {
	ui.buttons["save_live"] = ui.newButton(idSaveLive, "Save Current State...", 10, 10, 175, 30, hinst)
	ui.buttons["load_file"] = ui.newButton(idLoadFile, "Load State File...", 190, 10, 175, 30, hinst)
	ui.buttons["refresh"] = ui.newButton(idRefresh, "Refresh System State", 370, 10, 175, 30, hinst)
	ui.buttons["apply"] = ui.newButton(idApply, "Apply File State to Selected", 550, 10, 225, 30, hinst)
	ui.buttons["append"] = ui.newButton(idAppend, "Append Selected to Snapshot", 785, 10, 225, 30, hinst)

	ui.target = ui.newStatic(idTarget, "Target Snapshot: No file loaded", 10, 48, 1240, 22, hinst)
	ui.status = ui.newStatic(idStatus, "Ready", 10, 70, 1240, 22, hinst)
	ui.progress = ui.newProgress(idProgress, 10, 94, 1240, 18, hinst)
	procShowWindow.Call(uintptr(ui.progress), swHide)

	ui.tab = ui.newTab(idTab, 10, 120, 1240, 38, hinst)
	tabNames := []string{"Tab 1: Mismatch", "Tab 2: Live Only", "Tab 3: File Only", "Tab 4: Matched"}
	for i, name := range tabNames {
		text := windows.StringToUTF16Ptr(name)
		ti := tabItem{Mask: 0x0001, Text: text}
		procSendMessageW.Call(uintptr(ui.tab), tcmInsertItemW, uintptr(i), uintptr(unsafe.Pointer(&ti)))
	}
	for i := 0; i < 4; i++ {
		styleEx := uint32(lvsExFullRowSelect | lvsExGridLines | lvsExDoubleBuffer)
		if i == 0 || i == 1 {
			styleEx |= lvsExCheckBoxes
		}
		style := uint32(lvsReport | lvsShowSelAlways)
		if i == 2 || i == 3 {
			style |= lvsSingleSel
		}
		ui.lists[i] = ui.newList(idListBase+i, 10, 158, 1240, 530, hinst, style, styleEx)
		ui.setupColumns(ui.lists[i])
		if i != 0 {
			procShowWindow.Call(uintptr(ui.lists[i]), swHide)
		}
	}
	return nil
}

func (ui *UI) newButton(id int, label string, x, y, w, h int32, hinst windows.Handle) windows.Handle {
	return ui.createChild("BUTTON", label, wsChild|wsVisible|wsTabStop, x, y, w, h, windows.Handle(id), hinst)
}

func (ui *UI) newStatic(id int, label string, x, y, w, h int32, hinst windows.Handle) windows.Handle {
	return ui.createChild("STATIC", label, wsChild|wsVisible, x, y, w, h, windows.Handle(id), hinst)
}

func (ui *UI) newTab(id int, x, y, w, h int32, hinst windows.Handle) windows.Handle {
	return ui.createChild("SysTabControl32", "", wsChild|wsVisible|tcsTabs, x, y, w, h, windows.Handle(id), hinst)
}

func (ui *UI) newProgress(id int, x, y, w, h int32, hinst windows.Handle) windows.Handle {
	return ui.createChild("msctls_progress32", "", wsChild|wsVisible|pbsMarquee, x, y, w, h, windows.Handle(id), hinst)
}

func (ui *UI) newList(id int, x, y, w, h int32, hinst windows.Handle, style, styleEx uint32) windows.Handle {
	hwnd := ui.createChildEx(wsExClientEdge, "SysListView32", "", wsChild|wsVisible|style, x, y, w, h, windows.Handle(id), hinst)
	procSendMessageW.Call(uintptr(hwnd), lvmSetExtendedListViewStyle, uintptr(styleEx), uintptr(styleEx))
	return hwnd
}

func (ui *UI) createChild(className, label string, style uint32, x, y, w, h int32, id windows.Handle, hinst windows.Handle) windows.Handle {
	return ui.createChildEx(0, className, label, style, x, y, w, h, id, hinst)
}

func (ui *UI) createChildEx(ex uint32, className, label string, style uint32, x, y, w, h int32, id windows.Handle, hinst windows.Handle) windows.Handle {
	cls := windows.StringToUTF16Ptr(className)
	text := windows.StringToUTF16Ptr(label)
	hwnd, _, _ := procCreateWindowExW.Call(
		uintptr(ex),
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(text)),
		uintptr(style),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		uintptr(ui.hwnd), uintptr(id), uintptr(hinst), 0,
	)
	return windows.Handle(hwnd)
}

func (ui *UI) setupColumns(list windows.Handle) {
	headers := []struct {
		Name  string
		Width int32
	}{
		{"Service Name", 260},
		{"Display Name", 300},
		{"Live Status", 150},
		{"Live Startup Type", 190},
		{"Target (File)", 190},
	}
	for i, h := range headers {
		txt := windows.StringToUTF16Ptr(h.Name)
		col := lvColumn{
			Mask:    0x0001 | 0x0002,
			Fmt:     0,
			CX:      h.Width,
			Text:    txt,
			TextMax: int32(len(h.Name) + 1),
			SubItem: int32(i),
			Image:   -1,
			Order:   int32(i),
		}
		procSendMessageW.Call(uintptr(list), lvmInsertColumnW, uintptr(i), uintptr(unsafe.Pointer(&col)))
	}
}

func (ui *UI) wndProc(hwnd windows.Handle, msgID uint32, wParam, lParam uintptr) uintptr {
	switch msgID {
	case wmCommand:
		return ui.handleCommand(wParam, lParam)
	case wmNotify:
		return ui.handleNotify(lParam)
	case wmSize:
		ui.layout()
	case wmClose:
		if ui.callbacks.OnClose != nil {
			ui.callbacks.OnClose(ui)
		}
		procDestroyWindow.Call(uintptr(hwnd))
	case wmDestroy:
		procPostQuitMessage.Call(0)
	case wmNCDestroy:
		if ui.combo != 0 {
			procDestroyWindow.Call(uintptr(ui.combo))
			ui.combo = 0
		}
	case wmAppRunQueued:
		ui.drainQueue()
	}
	r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msgID), wParam, lParam)
	return r
}

func (ui *UI) handleCommand(wParam, lParam uintptr) uintptr {
	id := int(uint16(wParam & 0xFFFF))
	code := int(uint16((wParam >> 16) & 0xFFFF))
	if id == idCombo {
		switch code {
		case cmbSelChange:
			ui.commitCombo()
			return 0
		case 8: // CBN_KILLFOCUS
			ui.endCombo(false)
			return 0
		}
	}
	if lParam != 0 {
		switch id {
		case idSaveLive:
			ui.emitAction("save_live")
		case idLoadFile:
			ui.emitAction("load_file")
		case idRefresh:
			ui.emitAction("refresh")
		case idApply:
			ui.emitAction("apply")
		case idAppend:
			ui.emitAction("append")
		}
	}
	return 0
}

func pointerFromUintptr(u uintptr, anchor *uintptr) unsafe.Pointer {
	a := uintptr(unsafe.Pointer(anchor))
	if u >= a {
		return unsafe.Add(unsafe.Pointer(anchor), int(u-a))
	}
	return unsafe.Add(unsafe.Pointer(anchor), -int(a-u))
}

func (ui *UI) handleNotify(lParam uintptr) uintptr {
	hdr := (*nmhdr)(pointerFromUintptr(lParam, &lParam))
	if hdr == nil {
		return 0
	}
	if hdr.HwndFrom == ui.tab && hdr.Code == tcnSelChange {
		value, _, _ := procSendMessageW.Call(uintptr(ui.tab), tcmGetCurSel, 0, 0)
		tab := int(int32(value))
		ui.showTab(tab)
		if ui.callbacks.OnTabChanged != nil {
			ui.callbacks.OnTabChanged(tab, ui)
		}
		return 0
	}
	for i, list := range ui.lists {
		if hdr.HwndFrom != list {
			continue
		}

		switch hdr.Code {
		case lvnColumnClick:
			click := (*nmitemActivate)(pointerFromUintptr(lParam, &lParam))
			if click.ISubItem >= 0 && click.ISubItem < 5 {
				ui.sortRows(i, int(click.ISubItem))
			}
			return 0

		case nmDblClk:
			act := (*nmitemActivate)(pointerFromUintptr(lParam, &lParam))
			if act.IItem < 0 || act.IItem >= int32(len(ui.rows[i])) {
				return 0
			}
			if (i == 0 || i == 1 || i == 3) && act.ISubItem == 3 && ui.rows[i][act.IItem].Editable {
				ui.beginCombo(i, int(act.IItem))
			}
			return 0
		}
	}
	return 0
}

func (ui *UI) syncCheckedRows(tab int) {
	if tab < 0 || tab >= len(ui.lists) || (tab != 0 && tab != 1) {
		return
	}

	list := ui.lists[tab]
	for i := range ui.rows[tab] {
		state, _, _ := procSendMessageW.Call(
			uintptr(list),
			lvmGetItemState,
			uintptr(i),
			lvmItemStateChecked|lvmItemStateUnchecked,
		)
		ui.rows[tab][i].Checked = state&lvmItemStateChecked != 0
	}
}

func (ui *UI) sortRows(tab, column int) {
	if tab < 0 || tab >= len(ui.lists) || column < 0 || column >= 5 {
		return
	}

	ui.endCombo(false)
	ui.syncCheckedRows(tab)

	// Cycle:
	//   different column -> A-Z
	//   A-Z              -> Z-A
	//   Z-A              -> original/unsorted
	if ui.sortColumn[tab] != column {
		ui.sortColumn[tab] = column
		ui.sortOrder[tab] = 1
	} else {
		switch ui.sortOrder[tab] {
		case 1:
			ui.sortOrder[tab] = 2
		case 2:
			ui.sortOrder[tab] = 0
		default:
			ui.sortOrder[tab] = 1
		}
	}

	if ui.sortOrder[tab] == 0 {
		ui.rows[tab] = append([]UIRow(nil), ui.originalRows[tab]...)

		// Preserve the current checkbox state when restoring original order.
		checked := make(map[string]bool, len(ui.rows[tab]))
		for _, row := range ui.rows[tab] {
			checked[row.Key] = row.Checked
		}

		ui.rows[tab] = append([]UIRow(nil), ui.originalRows[tab]...)
		for i := range ui.rows[tab] {
			if checkedValue, ok := checked[ui.rows[tab][i].Key]; ok {
				ui.rows[tab][i].Checked = checkedValue
			}
		}
	} else {
		sort.SliceStable(ui.rows[tab], func(i, j int) bool {
			a := ui.sortValue(ui.rows[tab][i], column)
			b := ui.sortValue(ui.rows[tab][j], column)

			if ui.sortOrder[tab] == 1 {
				return strings.ToLower(a) < strings.ToLower(b)
			}
			return strings.ToLower(a) > strings.ToLower(b)
		})
	}

	ui.renderRows(tab)
}

func (ui *UI) sortValue(row UIRow, column int) string {
	switch column {
	case 0:
		return row.Key
	case 1:
		return row.DisplayName
	case 2:
		return row.Status
	case 3:
		return row.LiveStartup
	case 4:
		return row.Target
	default:
		return ""
	}
}

func (ui *UI) showTab(index int) {
	if index < 0 || index >= 4 {
		index = 0
	}
	for i, list := range ui.lists {
		cmd := swHide
		if i == index {
			cmd = swShown
		}
		procShowWindow.Call(uintptr(list), uintptr(cmd))
	}
}

func (ui *UI) layout() {
	var rc rect
	procGetClientRect.Call(uintptr(ui.hwnd), uintptr(unsafe.Pointer(&rc)))
	width := rc.Right - rc.Left
	height := rc.Bottom - rc.Top
	if width < 100 || height < 100 {
		return
	}
	procMoveWindow.Call(uintptr(ui.target), 10, 48, uintptr(width-20), 22, 1)
	procMoveWindow.Call(uintptr(ui.status), 10, 70, uintptr(width-20), 22, 1)
	procMoveWindow.Call(uintptr(ui.progress), 10, 94, uintptr(width-20), 18, 1)
	procMoveWindow.Call(uintptr(ui.tab), 10, 120, uintptr(width-20), 32, 1)
	procMoveWindow.Call(uintptr(ui.lists[0]), 10, 158, uintptr(width-20), uintptr(height-168), 1)
	for i := 1; i < 4; i++ {
		procMoveWindow.Call(uintptr(ui.lists[i]), 10, 158, uintptr(width-20), uintptr(height-168), 1)
	}
}

func (ui *UI) SetTabCounts(counts [4]int) {
	names := []string{"Tab 1: Mismatch", "Tab 2: Live Only", "Tab 3: File Only", "Tab 4: Matched"}
	for i, name := range names {
		text := windows.StringToUTF16Ptr(fmt.Sprintf("%s (%d)", name, counts[i]))
		ti := tabItem{Mask: 0x0001, Text: text}
		procSendMessageW.Call(uintptr(ui.tab), tcmFirst+61, uintptr(i), uintptr(unsafe.Pointer(&ti))) // TCM_SETITEMW
	}
}

func (ui *UI) ShowError(title string, err error) {
	if err == nil {
		return
	}
	body := windows.StringToUTF16Ptr(err.Error())
	caption := windows.StringToUTF16Ptr(title)
	procMessageBoxW.Call(uintptr(ui.hwnd), uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(caption)), 0x00000010)
}

func (ui *UI) SetStatus(text string) { ui.setText(ui.status, text) }
func (ui *UI) SetTarget(text string) { ui.setText(ui.target, text) }

func (ui *UI) setText(hwnd windows.Handle, text string) {
	if hwnd == 0 {
		return
	}
	p := windows.StringToUTF16Ptr(text)
	procSetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(p)))
}

func (ui *UI) CurrentTab() int {
	value, _, _ := procSendMessageW.Call(uintptr(ui.tab), tcmGetCurSel, 0, 0)
	return int(int32(value))
}

func (ui *UI) SetListsEnabled(enabled bool) {
	state := boolToUintptr(enabled)
	for _, list := range ui.lists {
		procEnableWindow.Call(uintptr(list), state)
	}
	procEnableWindow.Call(uintptr(ui.tab), state)
}

func (ui *UI) SetBusy(busy bool) {
	ui.busy = busy
	if busy {
		procShowWindow.Call(uintptr(ui.progress), swShown)
		procSendMessageW.Call(uintptr(ui.progress), pbmSetMarquee, 1, 30)
	} else {
		procSendMessageW.Call(uintptr(ui.progress), pbmSetMarquee, 0, 0)
		procShowWindow.Call(uintptr(ui.progress), swHide)
	}
}

func (ui *UI) Busy() bool {
	return ui.busy
}

func (ui *UI) SetActionEnabled(action string, enabled bool) {
	hwnd := ui.buttons[action]
	if hwnd == 0 {
		return
	}
	// BM_SETSTATE is intentionally not used; BM_SETSTATE only changes the
	// pushed visual state. EnableWindow is the correct state mutation.
	procEnableWindow.Call(uintptr(hwnd), boolToUintptr(enabled))
}

func (ui *UI) SetRows(tab int, rows []UIRow) {
	if tab < 0 || tab >= len(ui.lists) {
		return
	}

	ui.originalRows[tab] = append([]UIRow(nil), rows...)
	ui.rows[tab] = append([]UIRow(nil), rows...)
	ui.sortColumn[tab] = -1
	ui.sortOrder[tab] = 0

	ui.renderRows(tab)
}

func (ui *UI) renderRows(tab int) {
	if tab < 0 || tab >= len(ui.lists) {
		return
	}

	rows := ui.rows[tab]
	list := ui.lists[tab]
	procSendMessageW.Call(uintptr(list), lvmDeleteAllItems, 0, 0)

	for i, row := range rows {
		values := []string{row.Key, row.DisplayName, row.Status, row.LiveStartup, row.Target}
		text := windows.StringToUTF16Ptr(values[0])
		item := lvItem{Mask: lvifText | lvifParam, Item: int32(i), SubItem: 0, Text: text, TextMax: int32(len(values[0]) + 1), Param: uintptr(i)}
		procSendMessageW.Call(uintptr(list), lvmInsertItemW, 0, uintptr(unsafe.Pointer(&item)))
		for col := 1; col < len(values); col++ {
			p := windows.StringToUTF16Ptr(values[col])
			li := lvItem{Mask: lvifText, Item: int32(i), SubItem: int32(col), Text: p, TextMax: int32(len(values[col]) + 1)}
			procSendMessageW.Call(uintptr(list), lvmSetItemW, 0, uintptr(unsafe.Pointer(&li)))
		}
		if row.Checked && (tab == 0 || tab == 1) {
			state := lvItem{Mask: lvifState, Item: int32(i), State: lvmItemStateChecked, StateMask: lvmItemStateChecked | lvmItemStateUnchecked}
			procSendMessageW.Call(uintptr(list), lvmSetItemState, uintptr(i), uintptr(unsafe.Pointer(&state)))
		}
	}
}

func (ui *UI) CheckedKeys(tab int) []string {
	if tab < 0 || tab >= len(ui.lists) {
		return nil
	}
	list := ui.lists[tab]
	out := make([]string, 0)
	for i, row := range ui.rows[tab] {
		state, _, _ := procSendMessageW.Call(uintptr(list), lvmGetItemState, uintptr(i), lvmItemStateChecked|lvmItemStateUnchecked)
		if state&lvmItemStateChecked != 0 {
			out = append(out, row.Key)
		}
	}
	return out
}

func (ui *UI) beginCombo(tab, row int) {
	ui.endCombo(false)
	list := ui.lists[tab]
	var r rect
	// For LVM_GETSUBITEMRECT the input rectangle encodes the subitem number
	// in Left and LVIR_BOUNDS in Top.
	r.Left = 3
	r.Top = lvirBounds
	procSendMessageW.Call(uintptr(list), lvmGetSubItemRect, uintptr(row), uintptr(unsafe.Pointer(&r)))
	// The list view control's coordinates are client-relative. Convert the
	// upper-left point to the list view's parent coordinates by adding the
	// fixed list placement used by layout().
	var listRC rect
	procGetClientRect.Call(uintptr(list), uintptr(unsafe.Pointer(&listRC)))
	height := r.Bottom - r.Top
	if height <= 0 {
		height = 22
	}
	width := r.Right - r.Left
	if width < 90 {
		width = 150
	}
	hwnd := ui.createChildEx(wsExClientEdge, "COMBOBOX", "", wsChild|wsVisible|wsTabStop|cbsDropdownList, r.Left+10, r.Top+158, width, height+120, windows.Handle(idCombo), getModuleHandle())
	if hwnd == 0 {
		return
	}
	ui.combo = hwnd
	ui.comboTab = tab
	ui.comboRow = row
	// Populate canonical choices. Boot/System remain available for completeness.
	choices := []string{"Automatic", "AutomaticDelayed", "Manual", "Disabled", "Boot", "System"}
	for _, choice := range choices {
		p := windows.StringToUTF16Ptr(choice)
		procSendMessageW.Call(uintptr(hwnd), cbAddString, 0, uintptr(unsafe.Pointer(p)))
	}
	current := ui.rows[tab][row].LiveStartup
	index := 0
	for i, choice := range choices {
		if choice == current {
			index = i
			break
		}
	}
	procSendMessageW.Call(uintptr(hwnd), cbSetCurSel, uintptr(index), 0)
	// Make the overlay line up with the actual list-view location. The row
	// rectangle is client-relative to the list; its top already includes the
	// header offset. We therefore anchor at (list x + cell x, list y + cell y).
	procMoveWindow.Call(uintptr(hwnd), uintptr(10+r.Left), uintptr(158+r.Top), uintptr(width), uintptr(height+120), 1)
	procSetFocus.Call(uintptr(hwnd))
	_ = listRC
}

func (ui *UI) commitCombo() {
	if ui.combo == 0 {
		return
	}
	idx, _, _ := procSendMessageW.Call(uintptr(ui.combo), cbGetCurSel, 0, 0)
	choices := []string{"Automatic", "AutomaticDelayed", "Manual", "Disabled", "Boot", "System"}
	if idx >= uintptr(len(choices)) {
		ui.endCombo(false)
		return
	}
	value := choices[idx]
	tab, row := ui.comboTab, ui.comboRow
	service := ""
	if tab >= 0 && tab < 4 && row >= 0 && row < len(ui.rows[tab]) {
		service = ui.rows[tab][row].Key
		ui.rows[tab][row].LiveStartup = value
	}
	ui.endCombo(true)
	if service != "" && ui.callbacks.OnStartupChanged != nil {
		ui.callbacks.OnStartupChanged(tab, service, value, ui)
	}
}

func (ui *UI) endCombo(commit bool) {
	if ui.combo == 0 {
		return
	}
	h := ui.combo
	ui.combo = 0
	if !commit {
		procDestroyWindow.Call(uintptr(h))
		return
	}
	procDestroyWindow.Call(uintptr(h))
}

func (ui *UI) Post(fn func()) error {
	if fn == nil {
		return errors.New("nil UI callback")
	}
	select {
	case ui.queue <- fn:
	default:
		return errors.New("UI dispatch queue full")
	}
	if r, _, _ := procPostMessageW.Call(uintptr(ui.hwnd), wmAppRunQueued, 0, 0); r == 0 {
		return fmt.Errorf("PostMessageW: %w", windows.GetLastError())
	}
	return nil
}

func (ui *UI) drainQueue() {
	for {
		select {
		case fn := <-ui.queue:
			fn()
		default:
			return
		}
	}
}

func (ui *UI) emitAction(action string) {
	if ui.callbacks.OnAction != nil {
		ui.callbacks.OnAction(action, ui)
	}
}

func (ui *UI) PromptOpenJSON() (string, error) {
	return ui.fileDialog(false)
}

func (ui *UI) PromptSaveJSON(defaultName string) (string, error) {
	return ui.fileDialog(true, defaultName)
}

func (ui *UI) fileDialog(save bool, defaults ...string) (string, error) {
	buffer := make([]uint16, 32768)
	if len(defaults) > 0 && defaults[0] != "" {
		copy(buffer, windows.StringToUTF16(defaults[0]))
	}
	filter := windows.StringToUTF16("JSON Snapshot (*.json)\x00*.json\x00All Files (*.*)\x00*.*\x00\x00")
	title := windows.StringToUTF16("Select service state snapshot")
	ofn := openFileName{
		StructSize: uint32(unsafe.Sizeof(openFileName{})),
		Owner:      ui.hwnd,
		Filter:     &filter[0],
		File:       &buffer[0],
		MaxFile:    uint32(len(buffer)),
		Title:      &title[0],
		Flags:      ofnExplorer | ofnPathMustExist | ofnEnableSizing,
	}
	var ret uintptr
	if save {
		ofn.Flags |= ofnOverwritePrompt | ofnPathMustExist
		ret, _, _ = procGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	} else {
		ofn.Flags |= ofnFileMustExist
		ret, _, _ = procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	}
	if ret == 0 {
		ext, _, _ := procCommDlgExtendedError.Call()
		if ext == 0 {
			return "", nil
		}
		return "", fmt.Errorf("common file dialog failed: 0x%X", ext)
	}
	return windows.UTF16ToString(buffer), nil
}

func (ui *UI) ShowAuditReport(results []RemediationResult) {
	var success, failed []string
	for _, r := range results {
		if r.Err == nil {
			success = append(success, fmt.Sprintf("%s -> %s", r.ServiceName, r.TargetState))
		} else {
			failed = append(failed, fmt.Sprintf("%s: %v", r.ServiceName, r.Err))
		}
	}
	var b strings.Builder
	b.WriteString("Succeeded:\r\n")
	if len(success) == 0 {
		b.WriteString("(none)\r\n")
	}
	for _, s := range success {
		b.WriteString("  " + s + "\r\n")
	}
	b.WriteString("\r\nFailed:\r\n")
	if len(failed) == 0 {
		b.WriteString("(none)\r\n")
	}
	for _, f := range failed {
		b.WriteString("  " + f + "\r\n")
	}
	text := windows.StringToUTF16Ptr(b.String())
	cap2 := windows.StringToUTF16Ptr("Service State Audit")
	procMessageBoxW.Call(uintptr(ui.hwnd), uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(cap2)), 0x00000040)
}

func (ui *UI) Close() {
	if ui.shuttingDown {
		return
	}
	ui.shuttingDown = true
	procPostMessageW.Call(uintptr(ui.hwnd), wmClose, 0, 0)
}

func getModuleHandle() windows.Handle {
	h, _, _ := procGetModuleHandleW.Call(0)
	return windows.Handle(h)
}

func boolToUintptr(v bool) uintptr {
	if v {
		return 1
	}
	return 0
}

// EnableWindow is intentionally kept here, with the rest of the Win32 surface.
func init() {
	// Ensure these imports/constants are retained by the compiler when the
	// project is built with aggressive dead-code elimination.
	_ = procGetConsoleMode
	_ = procGetSubMenu
	_ = wmVScroll
	_ = wmHScroll
	_ = wsBorder
	_ = lvifImage
	_ = lvifState
	_ = lvifParam
	_ = lvmSetItemWCode
}

// The direct user32 call is kept private to this file; no Win32 identifier
// leaks into wtw.go or main.go.
var procEnableWindow = user32.NewProc("EnableWindow")
var procSetFocus = user32.NewProc("SetFocus")
var procShowWindowAsync = user32.NewProc("ShowWindowAsync")
