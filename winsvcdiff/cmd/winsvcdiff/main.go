//go:build windows && amd64

// Copyright 2026 workturnedplay
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"os"

	"winsvcdiff/pkg/diff"
	"winsvcdiff/pkg/elevation"
	"winsvcdiff/pkg/scm"
	"winsvcdiff/pkg/snapshot"
	"winsvcdiff/pkg/ui"
)

func main() {
	// 1. Enforce elevated token requirement
	if !elevation.IsElevated() {
		elevation.RelaunchElevated()
		return
	}

	// 2. Query live SCM state
	liveServices, err := scm.EnumerateWin32Services()
	if err != nil {
		fmt.Fprintf(os.Stderr, "SCM Enumeration Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully enumerated %d active Win32 services.\n", len(liveServices))

	// Example CLI execution path demonstrating core snapshot & diff engine workflow
	if len(os.Args) > 1 && os.Args[1] == "--export" {
		outPath := "Win11-Services-Snapshot.json"
		if len(os.Args) > 2 {
			outPath = os.Args[2]
		}
		err := snapshot.SaveSnapshot(outPath, liveServices)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Export failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Snapshot written to %s\n", outPath)
		return
	}

	var snap *snapshot.Snapshot
	if len(os.Args) > 1 && os.Args[1] == "--compare" {
		snapPath := os.Args[2]
		var loadErr error
		snap, loadErr = snapshot.LoadSnapshot(snapPath)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "Snapshot load error: %v\n", loadErr)
			os.Exit(1)
		}
	}

	diffs := diff.Categorize(snap, liveServices)

	fmt.Printf("\n--- Diff Summary ---\n")
	fmt.Printf("Tab 1 (Mismatched): %d\n", len(diffs.Mismatched))
	fmt.Printf("Tab 2 (Live Only)  : %d\n", len(diffs.LiveOnly))
	fmt.Printf("Tab 3 (File Only)  : %d\n", len(diffs.FileOnly))
	fmt.Printf("Tab 4 (Matched)    : %d\n", len(diffs.Matched))

	if len(diffs.Mismatched) > 0 && len(os.Args) > 3 && os.Args[3] == "--apply" {
		fmt.Println("\nApplying batch remediation to mismatched services...")
		batchRes := ui.ApplySelectedBatch(diffs.Mismatched)
		fmt.Printf("Batch applied. Succeeded: %d, Failed: %d\n", batchRes.Succeeded, len(batchRes.Failed))
		for _, f := range batchRes.Failed {
			fmt.Printf("  [FAIL] %s\n", f)
		}
	}

	fmt.Println("\nExecution finished. Press Enter to exit...")
	var dummy string
	fmt.Scanln(&dummy)
}
