@echo off
go env GOWORK
::go env -w GOWORK=off
set GOWORK=off
go env GOWORK

go vet ./...
pause
