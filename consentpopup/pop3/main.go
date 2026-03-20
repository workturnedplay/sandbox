package main

import (
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/webview/webview_go"
)

const (
	GWL_EXSTYLE       = -20
	WS_EX_NOACTIVATE  = 0x08000000
	WS_EX_TOPMOST     = 0x00000008
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	procGetWindowLongPtrW   = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW   = user32.NewProc("SetWindowLongPtrW")
)

func setNoActivate(hwnd uintptr) {
	style, _, _ := procGetWindowLongPtrW.Call(hwnd, uintptr(int32(GWL_EXSTYLE)))
	procSetWindowLongPtrW.Call(
		hwnd,
		uintptr(int32(GWL_EXSTYLE)),
		style|WS_EX_NOACTIVATE|WS_EX_TOPMOST,
	)
}

func main() {
	runtime.LockOSThread()

	w := webview.New(false)
	defer w.Destroy()

	w.SetTitle("DNS Request")
	w.SetSize(420, 200, webview.HintNone)

	html := `
<!doctype html>
<html>
<body style="font-family:sans-serif">
<h3>Allow DNS request?</h3>
<p id="host">example.com</p>

<button id="yes" disabled>Yes</button>
<button id="no" disabled>No</button>
<button id="lookup" disabled>Look it up</button>

<script>
setTimeout(() => {
  document.querySelectorAll("button").forEach(b => b.disabled = false);
}, 1000);

document.getElementById("lookup").onclick = () => {
  document.getElementById("host").innerText =
    "Resolving… (pretend lookup result)";
};
</script>
</body>
</html>
`
	w.Navigate("data:text/html," + urlEncode(html))

	// After window exists, patch styles
	go func() {
		time.Sleep(100 * time.Millisecond)
		hwnd := uintptr(w.Window())
		setNoActivate(hwnd)
	}()

	w.Run()
}

// minimal URL encoding (enough for demo)
func urlEncode(s string) string {
	out := ""
	for _, r := range s {
		if r == ' ' {
			out += "%20"
		} else {
			out += string(r)
		}
	}
	return out
}
