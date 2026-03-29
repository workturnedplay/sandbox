@echo off
set GOWORK=off
go env GOWORK
go test -v ./...
pause