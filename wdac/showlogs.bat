@echo off

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "Get-WinEvent -LogName 'Microsoft-Windows-CodeIntegrity/Operational' -MaxEvents 50"

pause