package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path != "/" {
        http.NotFound(w, r)
        return
    }
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		// Force chunked streaming so Firefox doesn't wait for the full body
		flusher, canFlush := w.(http.Flusher)

		line := strings.Repeat("A", 99) + "\n"
		for i := range 900_000 {
			fmt.Fprintf(w, "%07d %s", i, line)
			if canFlush && i%10 == 0 {
				flusher.Flush()
				log.Printf("flushed at line %d", i)
			}
		}
	})

	addr := "127.0.0.1:9999"
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("ListenAndServe: %v", err)
	}
}