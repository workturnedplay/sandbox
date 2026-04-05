rem set GODEBUG=gcshrinkstackoff=1
rem go build -gcflags="-m -m" main.go | findstr /C:"flags escapes"
rem pause
go run main.go
pause