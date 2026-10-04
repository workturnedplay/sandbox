package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const (
	bindAddr     = "127.81.21.31:80"
	readTimeout  = 5 * time.Second
	writeTimeout = 5 * time.Second
	idleTimeout  = 15 * time.Second
	maxBodyBytes = 64 * 1024 // 64 KB safety cap to prevent unbounded body reads
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Timestamp recorded at socket request execution time
		now := time.Now().UTC().Format(time.RFC3339Nano)

		// Read payload safely using LimitReader to avoid memory exhaustion
		var bodyStr string
		if r.Body != nil {
			defer r.Body.Close()
			bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
			if err != nil {
				bodyStr = fmt.Sprintf("<error reading body: %v>", err)
			} else if len(bodyBytes) == 0 {
				bodyStr = "<empty>"
			} else {
				bodyStr = string(bodyBytes)
			}
		} else {
			bodyStr = "<nil>"
		}

		// Format headers for log output
		var headerLines []string
		for name, values := range r.Header {
			headerLines = append(headerLines, fmt.Sprintf("    %s: %s", name, strings.Join(values, ", ")))
		}
		headersFormatted := strings.Join(headerLines, "\n")
		if len(headerLines) == 0 {
			headersFormatted = "    <none>"
		}

		// Direct stdout logging of identity, path, headers, and payload
		log.Printf(
			"\n[+] Incoming Probe\n"+
				"    Timestamp : %s\n"+
				"    RemoteAddr: %s\n"+
				"    Proto     : %s\n"+
				"    Method    : %s\n"+
				"    Host      : %s\n"+
				"    RequestURI: %s\n"+
				"    Headers   :\n%s\n"+
				"    Payload   : %s\n",
			now,
			r.RemoteAddr,
			r.Proto,
			r.Method,
			r.Host,
			r.URL.RequestURI(),
			headersFormatted,
			bodyStr,
		)

		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{
		Addr:              bindAddr,
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("[+] NCSI logging server listening on %s\n", bindAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[-] ListenAndServe failed: %v\n", err)
		}
	}()

	<-stop
	log.Println("[*] Shutdown signal received, terminating...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("[-] Graceful shutdown failed: %v\n", err)
	}

	log.Println("[+] Server stopped cleanly.")
}