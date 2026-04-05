echo The -l flag prevents inlining.
go run -gcflags="-l" "nottrue interface breaks uintptr liveness.go"
pause