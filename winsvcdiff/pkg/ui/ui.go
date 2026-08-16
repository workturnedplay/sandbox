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

package ui

import (
	"fmt"

	"golang.org/x/sys/windows"
	"winsvcdiff/pkg/diff"
	"winsvcdiff/pkg/scm"
	"winsvcdiff/pkg/snapshot"
)

type BatchApplyResult struct {
	Succeeded int
	Failed    []string
}

// ApplySelectedBatch iterates over selected Tab 1 items, mutating live state without aborting on individual error.
func ApplySelectedBatch(targets []diff.DiffItem) BatchApplyResult {
	res := BatchApplyResult{
		Failed: make([]string, 0),
	}

	for _, item := range targets {
		if item.TargetStartType == "Not in File" || item.TargetStartType == "N/A" {
			continue
		}

		err := scm.MutateService(item.ServiceName, item.LiveInstanceName, item.IsPerUser, item.TargetStartType)
		if err != nil {
			// Capture error code and continue batch loop
			winErr, ok := err.(windows.Errno)
			var errDetail string
			if ok {
				errDetail = fmt.Sprintf("%s: %s (0x%X)", item.ServiceName, winErr.Error(), uint32(winErr))
			} else {
				errDetail = fmt.Sprintf("%s: %v", item.ServiceName, err)
			}
			res.Failed = append(res.Failed, errDetail)
		} else {
			res.Succeeded++
		}
	}

	return res
}

// AppendLiveOnlyToSnapshot merges selected Tab 2 items into loaded snapshot buffer.
func AppendLiveOnlyToSnapshot(snap *snapshot.Snapshot, selected []diff.DiffItem, live map[string]scm.ServiceInfo) {
	if snap == nil || snap.Services == nil {
		return
	}

	for _, item := range selected {
		if liveInfo, ok := live[item.ServiceName]; ok {
			snap.Services[item.ServiceName] = snapshot.ServiceRecord{
				DisplayName:  liveInfo.DisplayName,
				StartType:    liveInfo.StartType,
				DelayedStart: liveInfo.DelayedStart,
				TriggerStart: liveInfo.TriggerStart,
				IsPerUser:    liveInfo.IsPerUser,
			}
		}
	}
}
