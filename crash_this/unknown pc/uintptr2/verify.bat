go build -gcflags="-m -m" "nottrue interface breaks uintptr liveness.go" 2>&1 | findstr /C:"size escapes"
rem go build -gcflags="-m -m" "nottrue interface breaks uintptr liveness.go" 2>&1
pause