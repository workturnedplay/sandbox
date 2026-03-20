@echo off
cd /d %~dp0
go run windows_usb_dev_restart_tool_go_no_cgo.go
pause