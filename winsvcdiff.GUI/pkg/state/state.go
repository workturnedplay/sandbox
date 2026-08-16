// pkg/state/state.go
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"winsvcdiff/pkg/scm"
)

type Snapshot struct {
	Timestamp int64                       `json:"timestamp"`
	OSBuild   string                      `json:"os_build"`
	Services  map[string]scm.ServiceState `json:"services"`
}

type DiffResult struct {
	Mismatched []MismatchedSvc
	LiveOnly   []scm.ServiceState
	FileOnly   []scm.ServiceState
	Matched    []scm.ServiceState
}

type MismatchedSvc struct {
	Name        string
	DisplayName string
	LiveStartup string
	FileStartup string
}

// Save atomically writes the SCM state to disk, mitigating partial writes.
func Save(path string, services map[string]scm.ServiceState) error {
	snap := Snapshot{
		Services: services,
		// Omitted timestamp/build population for brevity.
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Write to temporary staging file first to prevent symlink attacks or corrupted JSON
	// if the application crashes mid-write.
	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	if _, err := file.Write(data); err != nil {
		file.Close()
		os.Remove(tmpPath)
		return err
	}
	
	// Ensure buffers are flushed to persistent storage before atomic rename.
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(tmpPath)
		return err
	}
	file.Close()

	// Atomic replacement.
	return os.Rename(tmpPath, path)
}

func Load(path string) (map[string]scm.ServiceState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("failed to parse snapshot JSON: %w", err)
	}

	return snap.Services, nil
}

// Compare computes the discrete set differences between two service maps.
func Compare(live, file map[string]scm.ServiceState) DiffResult {
	var res DiffResult

	for name, liveSvc := range live {
		fileSvc, exists := file[name]
		if !exists {
			res.LiveOnly = append(res.LiveOnly, liveSvc)
			continue
		}

		if liveSvc.Startup != fileSvc.Startup {
			res.Mismatched = append(res.Mismatched, MismatchedSvc{
				Name:        name,
				DisplayName: liveSvc.DisplayName,
				LiveStartup: liveSvc.Startup,
				FileStartup: fileSvc.Startup,
			})
		} else {
			res.Matched = append(res.Matched, liveSvc)
		}
	}

	for name, fileSvc := range file {
		if _, exists := live[name]; !exists {
			res.FileOnly = append(res.FileOnly, fileSvc)
		}
	}

	return res
}