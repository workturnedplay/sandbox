@echo off
set VID=1532
set PID=0109


for /f "delims=" %%i in ('powershell -NoProfile -Command "pnputil /enum-devices /connected /class USB | Select-String 'VID_%VID%&PID_%PID%' | Where-Object { $_ -notmatch 'MI_' } | ForEach-Object { ($_ -split '\s+')[2] }"') do (
    set INSTANCE_ID=%%i
)

if "%INSTANCE_ID%"=="" (
    echo Device not found: VID_%VID% PID_%PID%
    pause
    exit /b 1
)

rem echo Restarting: %INSTANCE_ID%
rem pnputil /restart-device "%INSTANCE_ID%"
powershell -NoProfile -Command "$id = pnputil /enum-devices /connected /class USB | Select-String 'VID_%VID%&PID_%PID%' | Where-Object { $_ -notmatch 'MI_' } | ForEach-Object { ($_ -split '\s+')[2] }; if (-not $id) { Write-Host 'Device not found: VID_%VID% PID_%PID%'; exit 1 }; Write-Host \"Restarting: $id\"; pnputil /restart-device $id"
pause