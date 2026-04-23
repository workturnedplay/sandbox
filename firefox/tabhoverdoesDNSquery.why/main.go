// it's because of network.dns.echconfig.enabled is true in about:config it only does DNS type HTTPS request not actually connect to the host!
package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"time"
)

func handler(w http.ResponseWriter, r *http.Request) {
	// Generate a fake ETag based on the current time so it's always "new"
	currentETag := fmt.Sprintf("\"%d\"", time.Now().Unix())
	lastModified := time.Now().Format(http.TimeFormat)

	// Log the incoming request
	fmt.Printf("\n[LOG] %s %s %s\n", r.Method, r.URL.Path, r.Proto)
	for name, values := range r.Header {
		for _, value := range values {
			fmt.Printf("  %s: %s\n", name, value)
		}
	}

	// Set headers to bypass Firefox's internal cache
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Last-Modified", lastModified)
	w.Header().Set("ETag", currentETag)

	// Handle the specific favicon request
	if r.URL.Path == "/favicon.ico" || r.URL.Path == "/fake-icon.ico" {
		fmt.Println(">>> BROWSER REQUESTED THE ICON!")
		w.Header().Set("Content-Type", "image/x-icon")
		w.WriteHeader(http.StatusOK)
		// Sending a tiny 1x1 transparent pixel as a "fake" icon
		w.Write([]byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x18, 0x00, 0x30, 0x00, 0x00, 0x00, 0x16, 0x00, 0x00, 0x00})
		return
	}

	// Serve HTML that explicitly points to the icon
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `
			<!DOCTYPE html>
			<html>
			<head>
				<link rel="icon" href="/favicon.ico">
				<link rel="shortcut icon" href="/favicon.ico">
				<title>Firefox Probe</title>
			</head>
			<body>
				<h1>Monitoring Active</h1>
				<p>If Firefox is looking for metadata, it should show up in the console.</p>
			</body>
			</html>
		`)
}

func main() {
	// // Handler that logs the exact URL requested
	// http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	// fmt.Printf("[LOG] Browser requested: %s %s\n", r.Method, r.URL.Path)
	// w.WriteHeader(http.StatusNotFound) // We don't actually need to give it anything
	// })

	// http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	// 	// 1. Build a visual separator for the log
	// 	fmt.Printf("\n--- NEW REQUEST: %s ---\n", r.RemoteAddr)
	// 	fmt.Printf("%s %s %s\n", r.Method, r.URL.Path, r.Proto)

	// 	// 2. Log all headers
	// 	for name, values := range r.Header {
	// 		for _, value := range values {
	// 			fmt.Printf("%s: %s\n", name, value)
	// 		}
	// 	}

	// 	// 3. Send a valid response (200 OK) instead of 404
	// 	w.Header().Set("Content-Type", "text/plain")
	// 	fmt.Fprintln(w, "Intercepted by Go Monitor")

	// 	fmt.Println("--------------------------")
	// })

	http.HandleFunc("/", handler)

	// Setup server to listen on 443
	server := &http.Server{
		Addr: "127.0.0.88:443",
		// This allows us to ignore TLS certificate errors for testing
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
	}

	fmt.Printf("Server listening on https://%s\n", server.Addr)
	fmt.Println("Waiting for Firefox to hover...")

	// You need a cert.pem and key.pem in the same folder.
	// See the generation command below.
	log.Fatal(server.ListenAndServeTLS("cert.pem", "key.pem"))
}
