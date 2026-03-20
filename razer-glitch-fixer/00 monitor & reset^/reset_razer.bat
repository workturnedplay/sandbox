@echo off

echo.
echo Razer keyboard POWER CYCLE test to remove firmware glitch.
echo =================================


setlocal
:: If marker is set, we're elevated (prevents relaunch loop)
if "%__ELEVATED__%"=="1" goto :elevated

:: If already elevated, continue
rem Check membership of Administrators group using whoami - bad
::whoami /groups | findstr /i "S-1-5-32-544" >nul 2>&1
:: Check token elevation via PowerShell; returns 0 if elevated, 1 if not
::powershell -NoProfile -ExecutionPolicy Bypass -Command ^
::  "Write-Output (([Security.Principal.WindowsIdentity]::GetCurrent()).Owner -ne $null); exit(([Security.Principal.WindowsPrincipal]::new([Security.Principal.WindowsIdentity]::GetCurrent())).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator) -ne $false ? 0 : 1)" >nul 2>&1
::if %errorlevel%==0 goto :elevated
:: Check elevation using PowerShell reliably; exit code 0 => elevated
::powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  ::"exit((New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator) -eq $true)" >nul 2>&1
  rem Use TokenElevation via PowerShell: exit code 0 = elevated, 1 = not elevated
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$token = [System.IntPtr]::Zero; if (-not [Advapi32]::OpenProcessToken((GetCurrentProcess).Handle, 8, [ref]$token)) { exit 2 } ; $te=0; $size=4; if (-not [Advapi32]::GetTokenInformation($token, 20, [ref]$te, $size, [ref]$size)) { exit 3 } ; if ($te -eq 1) { exit 0 } else { exit 1 }" ^
  -CommandType Script -ErrorAction Stop >nul 2>&1
if %errorlevel%==0 (
  goto :elevated
)

:: Not elevated — relaunch elevated and set marker so child knows it's the elevated instance
echo "Re-running as admin..."
:: Re-run this batch elevated using PowerShell
set "script=%~f0"
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "Start-Process -FilePath 'cmd.exe' -ArgumentList '/c set \"__ELEVATED__=1\" && \"\"%script%\"\"' -Verb RunAs"
if %errorlevel% NEQ 0 (
echo This script needs to run as Administrator
pause
)
exit /b

:elevated
:: --- rest of your script here ---
echo Running elevated...
endlocal

rem your commands...
setlocal EnableDelayedExpansion




echo.

:: === CHANGE THESE TWO LINES TO YOUR ACTUAL VALUES ===

:: 1. The InstanceId of the PARENT USB device (usually starts with USB\VID_1532&PID_0109 without MI_xx)
::set "PARENT_INSTANCE=USB\VID_1532&PID_0109\6&32DE58AC&0"
::set "PARENT_INSTANCE=USB\VID_1532^&PID_0109^&6^&32DE58AC^&0"
:: the child - fail
::set "PARENT_INSTANCE=USB\VID_1532&PID_0109&MI_00\6&32DE58AC&0&0000"
:: the other child (works, but effect is unknown)
::set "PARENT_INSTANCE=USB\VID_1532&PID_0109&MI_01\6&1DBACD0E&0&0001
:: the parent - fail
::set "PARENT_INSTANCE=USB\VID_1532&PID_0109\5&1E7D8DB7&0&14"
:: the hub - fail
set "PARENT_INSTANCE=USB\ROOT_HUB30\4&186df573&0&0
::InstanceId   : USB\VID_1532&PID_0109&MI_00\6&1DBACD0E&0&0000
::InstanceId   : USB\VID_1532&PID_0109&MI_01\6&1DBACD0E&0&0001
::InstanceId   : USB\VID_1532&PID_0109&MI_00\6&32DE58AC&0&0000
::InstanceId   : USB\VID_1532&PID_0109\5&1E7D8DB7&0&6
::InstanceId   : USB\VID_1532&PID_0109&MI_01\6&32DE58AC&0&0001
::InstanceId   : USB\VID_1532&PID_0109\5&1E7D8DB7&0&14

:: 2. Optional friendly name check (just for logging / safety) — can be empty
set "EXPECTED_NAME=USB Input Device"

:: =====================================================

echo "Looking for parent device: %PARENT_INSTANCE%"
echo.

:: Quick check if the device even exists
powershell -NoProfile -Command ^
    "$dev = Get-PnpDevice -InstanceId '%PARENT_INSTANCE%' -ErrorAction SilentlyContinue; " ^
    "if ($dev) { " ^
    "  Write-Output 'Found device:'; " ^
    "  Write-Output ('  Name:       ' + $dev.FriendlyName); " ^
    "  Write-Output ('  Status:     ' + $dev.Status); " ^
    "  Write-Output ('  InstanceId: ' + $dev.InstanceId); " ^
    "} else { " ^
    "  Write-Output 'Device not found! Check InstanceId.'; " ^
    "  exit 1; " ^
    "}"

echo.
echo Disabling device in 1 seconds... (Ctrl+C to abort)
timeout /t 1 >nul

pnputil /disable-device "%PARENT_INSTANCE%"

::powershell -NoProfile -Command ^
::    "$ErrorActionPreference = 'Stop'; " ^
::    "Get-PnpDevice -InstanceId '%PARENT_INSTANCE%' | Disable-PnpDevice -Confirm:$false; " ^
::    "Write-Output 'Disable command sent.'"

echo.
echo Waiting 5 seconds for power-down...
timeout /t 5 >nul

echo.
echo Re-enabling device...
pnputil /enable-device "%PARENT_INSTANCE%"
::powershell -NoProfile -Command ^
::    "$ErrorActionPreference = 'Stop'; " ^
::    "Get-PnpDevice -InstanceId '%PARENT_INSTANCE%' | Enable-PnpDevice -Confirm:$false; " ^
::    "Write-Output 'Enable command sent.'"

echo.
echo Done.
echo Watch the keyboard LEDs - they should go off for a few seconds, then come back.
echo If the glitch stops after this, the reset worked.
pause