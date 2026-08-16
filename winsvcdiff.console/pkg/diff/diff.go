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

package diff

import (
	"winsvcdiff/pkg/scm"
	"winsvcdiff/pkg/snapshot"
)

type DiffItem struct {
	ServiceName      string
	DisplayName      string
	LiveStatus       string
	LiveStartType    string
	TargetStartType  string
	LiveInstanceName string
	IsPerUser        bool
}

type DiffResult struct {
	Mismatched []DiffItem // Tab 1
	LiveOnly   []DiffItem // Tab 2
	FileOnly   []DiffItem // Tab 3
	Matched    []DiffItem // Tab 4
}

// Categorize evaluate loaded snapshot against live SCM configuration.
func Categorize(snap *snapshot.Snapshot, live map[string]scm.ServiceInfo) DiffResult {
	res := DiffResult{
		Mismatched: make([]DiffItem, 0),
		LiveOnly:   make([]DiffItem, 0),
		FileOnly:   make([]DiffItem, 0),
		Matched:    make([]DiffItem, 0),
	}

	if snap == nil || snap.Services == nil {
		// No file loaded; all live services are LiveOnly (Tab 2)
		for name, l := range live {
			res.LiveOnly = append(res.LiveOnly, DiffItem{
				ServiceName:      name,
				DisplayName:      l.DisplayName,
				LiveStatus:       l.LiveStatus,
				LiveStartType:    l.StartType,
				TargetStartType:  "Not in File",
				LiveInstanceName: l.LiveInstanceName,
				IsPerUser:        l.IsPerUser,
			})
		}
		return res
	}

	// Track processed keys to evaluate FileOnly items
	processedFileKeys := make(map[string]bool, len(snap.Services))

	for liveName, l := range live {
		fileRec, existsInFile := snap.Services[liveName]

		if !existsInFile {
			// Tab 2: Live Only
			res.LiveOnly = append(res.LiveOnly, DiffItem{
				ServiceName:      liveName,
				DisplayName:      l.DisplayName,
				LiveStatus:       l.LiveStatus,
				LiveStartType:    l.StartType,
				TargetStartType:  "Not in File",
				LiveInstanceName: l.LiveInstanceName,
				IsPerUser:        l.IsPerUser,
			})
			continue
		}

		processedFileKeys[liveName] = true

		if l.StartType == fileRec.StartType {
			// Tab 4: Matched
			res.Matched = append(res.Matched, DiffItem{
				ServiceName:      liveName,
				DisplayName:      l.DisplayName,
				LiveStatus:       l.LiveStatus,
				LiveStartType:    l.StartType,
				TargetStartType:  fileRec.StartType,
				LiveInstanceName: l.LiveInstanceName,
				IsPerUser:        l.IsPerUser,
			})
		} else {
			// Tab 1: Mismatched
			res.Mismatched = append(res.Mismatched, DiffItem{
				ServiceName:      liveName,
				DisplayName:      l.DisplayName,
				LiveStatus:       l.LiveStatus,
				LiveStartType:    l.StartType,
				TargetStartType:  fileRec.StartType,
				LiveInstanceName: l.LiveInstanceName,
				IsPerUser:        l.IsPerUser,
			})
		}
	}

	// Tab 3: File Only (In snapshot file, absent from live SCM)
	for fileName, f := range snap.Services {
		if !processedFileKeys[fileName] {
			res.FileOnly = append(res.FileOnly, DiffItem{
				ServiceName:     fileName,
				DisplayName:     f.DisplayName,
				LiveStatus:      "N/A (Not Installed)",
				LiveStartType:   "N/A",
				TargetStartType: f.StartType,
				IsPerUser:       f.IsPerUser,
			})
		}
	}

	return res
}
