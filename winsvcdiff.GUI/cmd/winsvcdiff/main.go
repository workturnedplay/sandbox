package main

import (
	"log"
	"os"

	"winsvcdiff/pkg/elevation"
)

func main() {
	elevated, err := elevation.IsElevated()
	if err != nil {
		// Failsafe exit if we can't determine token privileges
		os.Exit(1)
	}

	if !elevated {
		err = elevation.RelaunchElevated()
		// If re-launch failed (user denied UAC or CreateProcess failure), exit silently.
		os.Exit(1)
	}

	// Execution reaches here only if running elevated under a HIGH_INTEGRITY token.
	// Proceed to UI Initialization, Snapshot Loading, and the Diff Engine.
	log.Println("Process elevated. Target Win32 SCM Engine initialized.")
	
	// TODO: Initialize Win32 Native UI Frame (requires extensive setup via x/sys/windows or external wrapper like 'walk')
	// TODO: Boot background Go routine for scm.EnumerateWin32Services()
	// TODO: Mount 4-Tab categorical diff engine.
}