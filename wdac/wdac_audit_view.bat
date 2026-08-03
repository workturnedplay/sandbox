@echo off
setlocal

echo ==========================================================
echo WDAC AUDIT - WOULD-BE BLOCKED EXECUTION EVENTS
echo ==========================================================
echo.

powershell -NoProfile -Command "$log='Microsoft-Windows-CodeIntegrity/Operational'; $ids=3076,3077,3089; Get-WinEvent -LogName $log -ErrorAction SilentlyContinue | Where-Object { $ids -contains $_.Id } | Sort-Object TimeCreated -Descending | ForEach-Object { Write-Host '----------------------------------------'; Write-Host ('Time: ' + $_.TimeCreated); Write-Host ('ID  : ' + $_.Id); Write-Host $_.Message }"

echo.
pause
endlocal