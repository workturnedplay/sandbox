@echo off
set GOWORK=off
go env GOWORK
set GODEBUG=gctrace=1 
go env GODEBUG
go test -v ./...
::go test -run TestGCUnsafe_Fail -count=1000
pause