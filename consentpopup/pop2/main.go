//go:build windows

package main

import (
	"fmt"
	"html"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2"
)

type Decision string

var (
	user32                = syscall.NewLazyDLL("user32.dll")
	procSetWindowLongPtrW = user32.NewProc("SetWindowLongPtrW")
	procGetWindowLongPtrW = user32.NewProc("GetWindowLongPtrW")
)

const (
	GWL_EXSTYLE        = -20
	WS_EX_TOPMOST     = 0x00000008
	WS_EX_TOOLWINDOW  = 0x00000080
	WS_EX_NOACTIVATE  = 0x08000000
)

func main() {
	ch := make(chan Decision, 1)

	go showConsentPopup("example.com", ch)

	fmt.Println("Decision:", <-ch)
}

func showConsentPopup(host string, ch chan Decision) {
	w := webview2.New(false)
	defer w.Destroy()

	// JS → Go bridge
	w.Bind("sendDecision", func(d string) {
		ch <- Decision(d)
		w.Terminate()
	})

	// Navigate safely (HTML-escaped)
	w.Navigate("data:text/html," + consentHTML(host))

	// Apply window styles AFTER creation
	hwnd := w.Window()
	exStyle, _, _ := procGetWindowLongPtrW.Call(hwnd, uintptr(GWL_EXSTYLE))

	newStyle := exStyle |
		WS_EX_TOPMOST |
		WS_EX_TOOLWINDOW |
		WS_EX_NOACTIVATE

	procSetWindowLongPtrW.Call(
		hwnd,
		uintptr(GWL_EXSTYLE),
		newStyle,
	)

	w.Run()
}


func consentHTML(host string) string {
	safeHost := html.EscapeString(host)

	return `
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<style>
body {
	font-family: system-ui, sans-serif;
	margin: 12px;
	background: #f3f3f3;
}
button {
	margin: 6px;
	padding: 6px 12px;
}
#buttons button {
	opacity: 0.5;
	pointer-events: none;
}
</style>
</head>
<body>

<p>Allow DNS request to <b>` + safeHost + `</b>?</p>

<div id="status"></div>

<div id="buttons">
	<button onclick="sendDecision('allow')">Allow</button>
	<button onclick="sendDecision('deny')">Deny</button>
	<button onclick="lookup()">Look it up</button>
	<button onclick="sendDecision('ignore')">Ignore</button>
</div>

<script>
setTimeout(() => {
	document.querySelectorAll("#buttons button").forEach(b => {
		b.style.opacity = 1;
		b.style.pointerEvents = "auto";
	});
}, 1000);

function lookup() {
	document.getElementById("status").innerText = "Looking up domain…";
	document.querySelectorAll("#buttons button").forEach(b => b.disabled = true);

	setTimeout(() => {
		document.getElementById("status").innerText =
			"Lookup complete. No known issues found.";
		document.querySelectorAll("#buttons button").forEach(b => b.disabled = false);
	}, 800);
}
</script>

</body>
</html>
`
}
