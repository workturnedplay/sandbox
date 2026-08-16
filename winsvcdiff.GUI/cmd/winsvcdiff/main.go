package main

import (
	"log"
	"winsvcdiff/pkg/ui"
)

func main() {
	// Initializes the Win32 message loop on the main thread.
	// OS threads are locked implicitly by the message pump blocking execution.
	if err := ui.Run(); err != nil {
		log.Fatalf("Fatal UI error: %v", err)
	}
}