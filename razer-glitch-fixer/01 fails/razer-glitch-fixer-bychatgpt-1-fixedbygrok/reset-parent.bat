@echo off
setlocal

echo Razer Parent Reset Test
echo =======================

set "PARENT=USB\VID_1532&PID_0109\5&1E7D8DB7&0&14"

echo "Using parent: %PARENT%"
echo.

powershell -NoProfile -Command ^
    "$ErrorActionPreference = 'Stop'; " ^
    "Write-Host 'Disabling parent...'; " ^
    "Get-PnpDevice -InstanceId '%PARENT%' | Disable-PnpDevice -Confirm:$false; " ^
    "Write-Host 'Disable sent.'"

echo Waiting 5 seconds (watch LEDs go off)...
timeout /t 5 >nul

powershell -NoProfile -Command ^
    "$ErrorActionPreference = 'Stop'; " ^
    "Write-Host 'Re-enabling parent...'; " ^
    "Get-PnpDevice -InstanceId '%PARENT%' | Enable-PnpDevice -Confirm:$false; " ^
    "Write-Host 'Enable sent.'"

echo.
echo Done. Watch if the keyboard LEDs turned off and back on.
pause