// cmd/winsvcdiff/main.go
package main

import (
	"log"
	"os"

	"winsvcdiff/pkg/elevation"
	"winsvcdiff/pkg/ui"
)

func main() {
	elevated, err := elevation.IsElevated()
	if err != nil {
		log.Fatalf("Failsafe exit: unable to determine token privileges: %v", err)
	}

	if !elevated {
		if err := elevation.RelaunchElevated(); err != nil {
			log.Printf("UAC re-launch request failed or denied: %v", err)
		}
		os.Exit(1)
	}

	// Execution reaches here only under a HIGH_INTEGRITY token.
	log.Println("Process elevated. Initializing native Win32 SCM engine and UI...")

  // Initializes the Win32 message loop on the main thread.
	// OS threads are locked implicitly by the message pump blocking execution.
	if err := ui.Run(); err != nil {
		log.Fatalf("Fatal UI loop error: %v", err)
	}
}