# Technical Specification: Win11 Service State Snapshot & Auditor (`win-svc-diff`)

**Target OS / Arch:** Windows 11 (`windows/amd64`)

**Language Toolchain:** Go (`Golang 1.26.2+`)

**Execution Privilege:** Mandatory Elevated (`Require Administrator`)

---

## 1. Lifecycle & Elevation Protocol

```
[ Process Launch ] 
       │
       ▼
┌────────────────────────────────┐
│ Check Token Privileges         │
│ (IsUserAnAdmin / Win32 API)    │
└───────────────┬────────────────┘
                │
        ┌───────┴───────┐
        │ Is Elevated?  │
        └───────┬───────┘
          No │     │ Yes
             │     ▼
             │   ┌────────────────────────────────┐
             │   │ Initialize GUI & Win32 SCM     │
             │   └────────────────────────────────┘
             ▼
┌────────────────────────────────┐
│ Re-launch via ShellExecuteW    │
│ lpOperation: "runas"           │
└───────────────┬────────────────┘
                │
        ┌───────┴───────┐
        │ UAC Accepted? │
        └───────┬───────┘
          No │     │ Yes
             │     ▼
             │   ┌────────────────────────────────┐
             │   │ Terminate Non-Elevated Parent  │
             │   │ (Child proceeds with GUI)      │
             │   └────────────────────────────────┘
             ▼
┌────────────────────────────────┐
│ Exit Process (Code 1)          │
│ Silence: No UI or Popups       │
└────────────────────────────────┘

```

### Privileged Execution Rules

* **Token Query:** Query privilege level via `IsUserAnAdmin()` or `GetTokenInformation(TokenElevation)`.
* **Silent UAC Elevation:** If running un-elevated, execute `ShellExecuteW(0, "runas", os.Executable(), args, cwd, SW_SHOWNORMAL)`.
* **User Refusal Safeguard:** If `ShellExecuteW` fails or returns `ERROR_CANCELLED` (`1223`), the process terminates immediately with exit code `1`. No error dialogs, terminal prompts, or partial UI frames are rendered.

---

## 2. Service Scope & Normalization

To enforce complete parity with `services.msc` and prevent hazardous modifications to low-level hardware or kernel drivers, SCM querying is strictly scoped to user-mode Win32 services.

### SCM Bitmask Filter

When calling `EnumServicesStatusExW`, set `dwServiceType` to:

```go
const SERVICE_WIN32_MASK = 0x00000030 // SERVICE_WIN32_OWN_PROCESS | SERVICE_WIN32_SHARE_PROCESS

```

* **Included:** Standalone executables, shared `svchost.exe` process groups, and per-user service instances (`SERVICE_USER_SERVICE` / `SERVICE_USERSERVICE_INSTANCE`).
* **Excluded:** Kernel drivers (`0x01`), File system drivers (`0x02`), Recognizer drivers (`0x04`).

### Per-User Service Identification (`_XXXXX` Suffix Normalization)

Windows 11 dynamically spawns per-user service instances at user logon with a random 5-to-8 character hexadecimal suffix (e.g., `BluetoothUserService_a1b2c`, `CBDhSvc_a1b2c`).

```
Live SCM Service Instance:   "BluetoothUserService_a1b2c"
                              │                   │
                              ▼                   ▼
                       Base Template Key     Session Suffix
                     "BluetoothUserService"    "_a1b2c"
                              │
                              ▼
Normalized Snapshot Key:     "BluetoothUserService"  (is_per_user: true)

```

1. **Detection Rule:** Classified as a per-user service if **either**:
* The registry key `HKLM\SYSTEM\CurrentControlSet\Services\<ServiceName>` contains a non-zero DWORD `UserServiceFlags`.
* The live service name matches `^(.+)_([0-9a-fA-F]{5,8})$` **AND** the extracted base name exists under `HKLM\SYSTEM\CurrentControlSet\Services\<BaseTemplate>`.


2. **Storage:** Strips the dynamic `_XXXXX` suffix and writes the record under the base key (`BluetoothUserService`). Sets `"is_per_user": true` in the snapshot schema.
3. **Diff Mapping:** Any active instance matching `BaseTemplate_LUID` maps directly to `BaseTemplate` in the snapshot buffer.
4. **Dual-Target Remediation:** When applying startup changes to a per-user service, updates are applied via `ChangeServiceConfigW` to **both**:
* The currently active instance (`BluetoothUserService_a1b2c`) via SCM.
* The base template service key (`BluetoothUserService`) so subsequent user logons inherit the target configuration.



---

## 3. Data Schema Specifications (`.json` v1.2)

Snapshot files must output deterministic, human-readable JSON formatted with 2-space indentation, sorted alphabetically by Service Name key.

```json
{
  "schema_version": "1.2",
  "exported_at": "2026-08-16T22:00:00Z",
  "system_info": {
    "hostname": "WIN11-PRO-01",
    "os_build": "10.0.22631"
  },
  "services": {
    "BluetoothUserService": {
      "display_name": "Bluetooth User Support Service",
      "start_type": "Manual",
      "delayed_start": false,
      "trigger_start": true,
      "is_per_user": true
    },
    "wuauserv": {
      "display_name": "Windows Update",
      "start_type": "Disabled",
      "delayed_start": false,
      "trigger_start": false,
      "is_per_user": false
    }
  }
}

```

### Canonical Startup Type Mappings

| Win32 `dwStartType` / Flag | Canonical Enum String | `services.msc` Equivalent |
| --- | --- | --- |
| `SERVICE_DISABLED (0x04)` | `"Disabled"` | Disabled |
| `SERVICE_DEMAND_START (0x03)` | `"Manual"` | Manual |
| `SERVICE_AUTO_START (0x02)` (`Delayed=0`) | `"Automatic"` | Automatic |
| `SERVICE_AUTO_START (0x02)` (`Delayed=1`) | `"AutomaticDelayed"` | Automatic (Delayed Start) |
| `SERVICE_BOOT_START (0x00)` | `"Boot"` | Boot |
| `SERVICE_SYSTEM_START (0x01)` | `"System"` | System |

---

## 4. Four-Tab Categorization Engine

The diff engine partitions services into four mutually exclusive tabs by comparing the loaded JSON snapshot against live SCM queries.

```
                       ┌──────────────────────────────────────┐
                       │  Loaded Snapshot File (.json)        │
                       │             vs.                      │
                       │  Live System SCM (SERVICE_WIN32_*)   │
                       └──────────────────┬───────────────────┘
                                          │
        ┌───────────────────┬─────────────┴───────┬──────────────────┐
        ▼                   ▼                     ▼                  ▼
┌──────────────┐    ┌──────────────┐      ┌──────────────┐   ┌──────────────┐
│    Tab 1     │    │    Tab 2     │      │    Tab 3     │   │    Tab 4     │
│  Mismatched  │    │  Live Only   │      │  File Only   │   │   Matched    │
├──────────────┤    ├──────────────┤      ├──────────────┤   ├──────────────┤
│ In File: YES │    │ In File: NO  │      │ In File: YES │   │ In File: YES │
│ In Live: YES │    │ In Live: YES │      │ In Live: NO  │   │ In Live: YES │
│ Config Match:│    │ (New service │      │ (Missing from│   │ Config Match:│
│ NO           │    │  on system)  │      │  this OS)    │   │ YES          │
└──────────────┘    └──────────────┘      └──────────────┘   └──────────────┘

```

### Categorization Matrix & Tab Rules

| Tab ID | Tab Name | Inclusion Criteria | Allowed User Operations |
| --- | --- | --- | --- |
| **Tab 1** | **Mismatched** | Exists in File AND Live SCM, but `start_type`, `delayed_start`, or `trigger_start` differ. | • Batch selection checkboxes to apply target file state.<br>

<br>• **Live Startup Type** dropdown override. |
| **Tab 2** | **Live Only** | Exists in Live SCM, but missing from File. *(Identifies new services added since snapshot).* | • **Live Startup Type** dropdown override.<br>

<br>• "Append Selected to Snapshot" export action. |
| **Tab 3** | **File Only** | Exists in File, but missing from Live SCM. *(Services not present on current OS build).* | • Read-only view of target configuration.<br>

<br>• Cannot modify live SCM (service non-existent). |
| **Tab 4** | **Matched** | Exists in File AND Live SCM with identical startup configuration. | • Read-only comparative overview.<br>

<br>• **Live Startup Type** dropdown override. |

---

## 5. UI Layout & Non-Destructive Live Overrides

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│ [ Save Current State... ] [ Load State File... ] [ ↻ Refresh System State ]             │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ Target Snapshot: Win11-Debloated.json (Services in File: 212)                           │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ [ Tab 1: Mismatch (4) ] [ Tab 2: Live Only (12) ] [ Tab 3: File Only (3) ] [ Tab 4... ] │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ [x] Service Name │ Display Name     │ Live Status │ Live Startup Type  │ Target (File)  │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ [x] wuauserv     │ Windows Update   │ Running     │ [ Automatic     ▼] │ Disabled       │
│ [x] DiagTrack    │ Connected User.. │ Stopped     │ [ Automatic (D) ▼] │ Disabled       │
│ [ ] Spooler      │ Print Spooler    │ Stopped     │ [ Disabled      ▼] │ Automatic      │
│ [x] WaaSMedicSvc │ WaaSMedicSvc     │ Running     │ [ Manual        ▼] │ Disabled       │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│ Selected: 3 services                                  [ Apply File State to Selected ]  │
└─────────────────────────────────────────────────────────────────────────────────────────┘

```

### Live Overrides Engine & Buffer Mechanics

1. **Interactive Column:** Rows in Tabs 1, 2, and 4 present a **Live Startup Type** combobox.
2. **Immediate SCM Mutation:** Modifying the dropdown immediately triggers `ChangeServiceConfigW` / `ChangeServiceConfig2W` on the live operating system.
3. **Buffer Isolation:** **The loaded in-memory file snapshot remains unmodified.**
4. **Dynamic Rescan Migration:** On post-mutation SCM re-query:
* A service in **Tab 1 (Mismatched)** updated to match the file state automatically migrates to **Tab 4 (Matched)**.
* A service in **Tab 4 (Matched)** manually changed away from the file state automatically migrates to **Tab 1 (Mismatched)**.
* A service in **Tab 2 (Live Only)** updates its live configuration while remaining in Tab 2.


5. **Live Status Column:** Displays state sourced from `SERVICE_STATUS_PROCESS.dwCurrentState` (`Running`, `Stopped`, `Paused`, `Start Pending`, `Stop Pending`).

---

## 6. Win32 SCM Mutator & Remediation Engine

```go
type RemediationResult struct {
    ServiceName string
    TargetState string
    Err         error
}

func SetServiceStartup(scmHandle windows.Handle, serviceName string, targetState string) error {
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
    }

    // 1. Mutate base dwStartType
    err = windows.ChangeServiceConfig(hSvc, windows.SERVICE_NO_CHANGE, dwStartType, windows.SERVICE_NO_CHANGE, nil, nil, nil, nil, nil, nil, nil)
    if err != nil {
        return err
    }

    // 2. Explicitly sync SERVICE_CONFIG_DELAYED_AUTO_START_INFO
    // CRITICAL: Must explicitly pass FALSE when switching away from Automatic (Delayed)
    // to clear orphaned 'DelayedAutostart = 1' registry entries.
    var delayedInfo struct {
        fDelayedAutostart uint32
    }
    if isDelayed {
        delayedInfo.fDelayedAutostart = 1
    } else {
        delayedInfo.fDelayedAutostart = 0
    }

    return windows.ChangeServiceConfig2(hSvc, windows.SERVICE_CONFIG_DELAYED_AUTO_START_INFO, (*byte)(unsafe.Pointer(&delayedInfo)))
}

```

### Batch Fault Isolation & Post-Batch Protocol

* **Non-Blocking Fault Isolation:** Batch execution loops iterate through every selected item in Tab 1. If an individual service fails (e.g., `ERROR_ACCESS_DENIED` `0x5` on `TrustedInstaller`-protected services like `WaaSMedicSvc` or `WinDefend`), the error is logged into a execution map, and the loop continues without aborting.
* **SCM Rescan:** Executes a full SCM re-enumeration immediately following batch completion.
* **Audit Report:** Displays a modal dialog detailing:
* **Succeeded:** Migrated automatically to Tab 4 (Matched).
* **Failed:** Retained in Tab 1 (Mismatched) with itemized Win32 failure reasons (e.g., `WaaSMedicSvc: Access is denied. (0x5)`).



---

## 7. Execution Threading & Application Manifest

### Concurrency Architecture

* **Off-Thread SCM Operations:** SCM queries (`EnumServicesStatusExW`, `QueryServiceConfig2W`) and batch updates run on dedicated background goroutines outside the UI thread.
* **UI Responsiveness:** Displays an indeterminate progress indicator during SCM rescan cycles, preventing "Not Responding" GUI frame freezes.

### Embedded Application Manifest (`app_manifest.xml`)

The compiled binary must embed a side-by-side Win32 manifest targeting:

```xml
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity version="1.0.0.0" processorArchitecture="amd64" name="win-svc-diff" type="win32"/>
  <dependency>
    <dependentAssembly>
      <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
    </dependentAssembly>
  </dependency>
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2, PerMonitor</dpiAwareness>
    </windowsSettings>
  </application>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security>
      <requestedPrivileges>
        <requestedExecutionLevel level="highestAvailable" uiAccess="false"/>
      </requestedPrivileges>
    </security>
  </trustInfo>
</assembly>

```

---

## 8. Export Workflows & Directory Structure

### Export Modes

1. **Save Live System State:** Exports all active `SERVICE_WIN32_*` instances to a new `.json` file.
2. **Export Live Only (Tab 2):** Exports only newly discovered services on the current machine.
3. **Export Merged Snapshot:** Combines the loaded snapshot buffer with selected items from Tab 2, generating an updated deployment baseline.

### Repository Architecture

```
win-svc-diff/
├── cmd/
│   └── winsvcdiff/
│       ├── main.go            # Entrypoint, elevation checks, app loop
│       └── app_manifest.xml   # Embedded Windows manifest (amd64, Common-Controls v6, DPI)
├── pkg/
│   ├── elevation/             # Token checks & ShellExecuteW elevation logic
│   ├── scm/                   # SCM Win32 API bindings, enumeration, and ChangeServiceConfig
│   ├── diff/                  # Categorization engine (4-Tab state evaluator)
│   ├── snapshot/              # JSON schema v1.2 import, export, and sorting
│   └── ui/                    # GUI renderer, tab controls, comboboxes, and dialogs
├── go.mod
└── go.sum

```