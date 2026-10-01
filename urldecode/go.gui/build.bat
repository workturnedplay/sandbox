@echo off
rem 1. Prevent the current working directory from taking precedence over PATH, doesn't work with eg. "start go.exe"
set "NoDefaultCurrentDirectoryInExePath=1"

setlocal enabledelayedexpansion

::if running as admin must get back to current dir:
cd /d %~dp0


rem go build -ldflags="-s -w" -o urldecode.exe main.go
go build -ldflags="-s -w -H=windowsgui" -o urldecode_gui.exe main.go
pause