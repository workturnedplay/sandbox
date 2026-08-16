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

package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"golang.org/x/sys/windows"
	"winsvcdiff/pkg/scm"
)

type SystemInfo struct {
	Hostname string `json:"hostname"`
	OSBuild  string `json:"os_build"`
}

type ServiceRecord struct {
	DisplayName  string `json:"display_name"`
	StartType    string `json:"start_type"`
	DelayedStart bool   `json:"delayed_start"`
	TriggerStart bool   `json:"trigger_start"`
	IsPerUser    bool   `json:"is_per_user"`
}

type Snapshot struct {
	SchemaVersion string                   `json:"schema_version"`
	ExportedAt    string                   `json:"exported_at"`
	SystemInfo    SystemInfo               `json:"system_info"`
	Services      map[string]ServiceRecord `json:"services"`
}

// SaveSnapshot exports live service map to a sorted JSON file.
func SaveSnapshot(filePath string, liveServices map[string]scm.ServiceInfo) error {
	hostname, _ := os.Hostname()
	buildStr := getOSBuildNumber()

	records := make(map[string]ServiceRecord, len(liveServices))
	for name, svc := range liveServices {
		records[name] = ServiceRecord{
			DisplayName:  svc.DisplayName,
			StartType:    svc.StartType,
			DelayedStart: svc.DelayedStart,
			TriggerStart: svc.TriggerStart,
			IsPerUser:    svc.IsPerUser,
		}
	}

	snap := Snapshot{
		SchemaVersion: "1.2",
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
		SystemInfo: SystemInfo{
			Hostname: hostname,
			OSBuild:  buildStr,
		},
		Services: records,
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON serialization failed: %w", err)
	}

	return os.WriteFile(filePath, data, 0644)
}

// LoadSnapshot reads and parses a snapshot JSON file.
func LoadSnapshot(filePath string) (*Snapshot, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var snap Snapshot
	err = json.Unmarshal(data, &snap)
	if err != nil {
		return nil, fmt.Errorf("invalid snapshot schema: %w", err)
	}

	return &snap, nil
}

func getOSBuildNumber() string {
	version := windows.RtlGetVersion()
	if version == nil {
		return "10.0.22631"
	}
	return fmt.Sprintf("%d.%d.%d", version.MajorVersion, version.MinorVersion, version.BuildNumber)
}

// GetSortedKeys returns service dictionary keys in strict alphabetical order.
func GetSortedKeys(m map[string]ServiceRecord) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
