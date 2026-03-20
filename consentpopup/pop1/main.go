//go:build windows

package main

import (
	"fmt"
	"time"

	"github.com/jchv/go-webview2"
)

type Decision string

func main() {
	decisionCh := make(chan Decision, 1)

	go func() {
		showConsentPopup("example.com", decisionCh)
	}()

	// Wait (or remove this line to not wait)
	decision := <-decisionCh
	fmt.Println("Decision:", decision)
}

func showConsentPopup(host string, ch chan Decision) {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: false,
		WindowOptions: webview2.WindowOptions{
			Width:  360,
			Height: 200,
			Title:  "DNS Request",
			TopMost: true,
			NoActivate: true,
		},
	})
	defer w.Destroy()

	// JS → Go
	w.Bind("sendDecision", func(d string) {
		ch <- Decision(d)
		w.Terminate()
	})

	html := consentHTML(host)
	w.Navigate("data:text/html," + html)

	w.Run()
}

func consentHTML(host string) string {
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

<p>Allow DNS request to <b>` + host + `</b>?</p>

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
