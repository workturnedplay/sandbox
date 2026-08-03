ollama run my-g4-8k --think=high --truncate=false --verbose
pause
exit

i've this go lang code:
ret, _, callErr := procSetConsoleTextAttribute.Call(h2, color)
	if callErr != nil || ret != 0 {
		err := windows.Errno(ret)
		panic(fmt.Errorf("SetConsoleTextAttribute failed, ret: %d err:%w, callErr:%v", ret, err, callErr))
	}

and i get a vscode warning  on that callErr arg for Errorf like this:
non-wrapping format verb for fmt.Errorf. Use `%w` to format errors (errorlint)
So I want to get rid of the warning, but not remove any of the things that i want to be part of the Errorf text.