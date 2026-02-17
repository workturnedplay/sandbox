
## spy--

Originally located here: https://github.com/workturnedplay/spy--  
(unless you got it from a fork, try `git remote -v` to check)  

`spy--` is a small Windows utility that I use on win11 instead of [spy++](https://github.com/westoncampbell/SpyPlusPlus/tree/master/spy17.0.34511.75) whenever I want to see what messages(eg. WM_PAINT) does a target window get.

The goal is to avoid the case where spy++ would crash and then require a full OS restart to get rid of its injected .dll which would otherwise cause any subsequently restarted spy++ and attempts to read the messages of a window to hang that window for a few seconds before giving up.

`spy--` was build with the help of AI (Grok specifically).

---

### Build

#### Requirements

You need `go.exe` of Go language to compile this code into a standalone exe.  
No internet required to compile, if you have Go already installed.  

#### Compile into .exe

Standard Go build:

```
go build
```

Or try `build.bat`.  
That gives you an `.exe` that you can run or try `run.bat`.  

---

### License

Licensed under the **Apache License, Version 2.0**.
See `LICENSE` for details.

---

## Third-party code

This repository includes vendored third-party Go modules under the `vendor/` directory so it can be built without internet access.

Those components are licensed under their respective licenses.
Individual license texts and notices are preserved alongside the vendored code.

