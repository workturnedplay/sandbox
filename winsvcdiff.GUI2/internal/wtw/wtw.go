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

package wtw

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = "1.2"

var perUserSuffixRE = regexp.MustCompile(`^(.+)_([0-9a-fA-F]{5,8})$`)

// ServiceConfig is the canonical, Win32-independent representation used by
// both live SCM state and snapshot state.
type ServiceConfig struct {
	DisplayName  string `json:"display_name"`
	StartType    string `json:"start_type"`
	DelayedStart bool   `json:"delayed_start"`
	TriggerStart bool   `json:"trigger_start"`
	IsPerUser    bool   `json:"is_per_user"`
}

// LiveService contains configuration plus current runtime state. Instances
// contains the actual SCM names that must be mutated when a normalized
// per-user service has one or more active instances.
type LiveService struct {
	Name          string
	Config        ServiceConfig
	CurrentStatus string
	Instances     []string
}

type Snapshot struct {
	SchemaVersion string                   `json:"schema_version"`
	ExportedAt    string                   `json:"exported_at"`
	SystemInfo    SystemInfo               `json:"system_info"`
	Services      map[string]ServiceConfig `json:"services"`
}

type SystemInfo struct {
	Hostname string `json:"hostname"`
	OSBuild  string `json:"os_build"`
}

type Category int

const (
	Mismatched Category = iota
	LiveOnly
	FileOnly
	Matched
)

func (c Category) String() string {
	switch c {
	case Mismatched:
		return "Mismatched"
	case LiveOnly:
		return "Live Only"
	case FileOnly:
		return "File Only"
	case Matched:
		return "Matched"
	default:
		return "Unknown"
	}
}

type DiffEntry struct {
	Name   string
	Target *ServiceConfig
	Live   *LiveService
}

type Diff struct {
	Mismatched []DiffEntry
	LiveOnly   []DiffEntry
	FileOnly   []DiffEntry
	Matched    []DiffEntry
}

type RemediationResult struct {
	ServiceName string
	TargetState string
	Err         error
}

// NewLiveSnapshot creates the v1.2 file representation from a live query.
func NewLiveSnapshot(live map[string]LiveService, system SystemInfo, now time.Time) Snapshot {
	services := make(map[string]ServiceConfig, len(live))
	for name, svc := range live {
		cfg := svc.Config
		cfg.IsPerUser = svc.Config.IsPerUser
		services[name] = cfg
	}
	return Snapshot{
		SchemaVersion: SchemaVersion,
		ExportedAt:    now.UTC().Format(time.RFC3339),
		SystemInfo:    system,
		Services:      services,
	}
}

func ValidateSnapshot(s Snapshot) error {
	if s.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version %q (expected %q)", s.SchemaVersion, SchemaVersion)
	}
	if s.Services == nil {
		return errors.New("snapshot services object is missing")
	}
	for name, svc := range s.Services {
		if strings.TrimSpace(name) == "" {
			return errors.New("snapshot contains an empty service name")
		}
		if err := ValidateStartType(svc.StartType); err != nil {
			return fmt.Errorf("service %q: %w", name, err)
		}
	}
	return nil
}

func LoadSnapshot(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read snapshot %q: %w", path, err)
	}
	var snapshot Snapshot
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("parse snapshot %q: %w", path, err)
	}
	if err := ValidateSnapshot(snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("invalid snapshot %q: %w", path, err)
	}
	return snapshot, nil
}

func SaveSnapshot(path string, snapshot Snapshot) error {
	if err := ValidateSnapshot(snapshot); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	data = append(data, '\n')
	if err := atomicWrite(path, data); err != nil {
		return err
	}
	return nil
}

func atomicWrite(path string, data []byte) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	f, err := os.CreateTemp(dir, ".win-svc-diff-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary snapshot: %w", err)
	}
	tmp := f.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(tmp)
		}
	}()
	if err := f.Chmod(0o644); err != nil {
		_ = f.Close()
		return fmt.Errorf("chmod temporary snapshot: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write temporary snapshot: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync temporary snapshot: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temporary snapshot: %w", err)
	}
	if err := os.Rename(tmp, abs); err != nil {
		return fmt.Errorf("replace snapshot: %w", err)
	}
	keep = true
	return nil
}

func DiffSnapshots(snapshot Snapshot, live map[string]LiveService) Diff {
	d := Diff{}
	names := make(map[string]struct{}, len(snapshot.Services)+len(live))
	for name := range snapshot.Services {
		names[name] = struct{}{}
	}
	for name := range live {
		names[name] = struct{}{}
	}

	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)

	for _, name := range ordered {
		target, inFile := snapshot.Services[name]
		liveSvc, inLive := live[name]
		switch {
		case inFile && inLive:
			if ConfigEqual(target, liveSvc.Config) {
				d.Matched = append(d.Matched, DiffEntry{Name: name, Target: &target, Live: &liveSvc})
			} else {
				d.Mismatched = append(d.Mismatched, DiffEntry{Name: name, Target: &target, Live: &liveSvc})
			}
		case inFile:
			targetCopy := target
			d.FileOnly = append(d.FileOnly, DiffEntry{Name: name, Target: &targetCopy})
		case inLive:
			liveCopy := liveSvc
			d.LiveOnly = append(d.LiveOnly, DiffEntry{Name: name, Live: &liveCopy})
		}
	}
	return d
}

func ConfigEqual(a, b ServiceConfig) bool {
	return a.StartType == b.StartType &&
		a.DelayedStart == b.DelayedStart &&
		a.TriggerStart == b.TriggerStart
}

func ValidateStartType(startType string) error {
	switch startType {
	case "Disabled", "Manual", "Automatic", "AutomaticDelayed", "Boot", "System":
		return nil
	default:
		return fmt.Errorf("invalid start_type %q", startType)
	}
}

func IsPerUserCandidate(serviceName string) (base string, ok bool) {
	m := perUserSuffixRE.FindStringSubmatch(serviceName)
	if len(m) != 3 || m[1] == "" {
		return "", false
	}
	return m[1], true
}

// NormalizeLiveServices folds dynamic per-user service instance names into
// their template key. Registry-backed detection is performed in main_api.go;
// this function only applies the already-determined normalized identity.
func NormalizeLiveServices(raw []LiveService) map[string]LiveService {
	out := make(map[string]LiveService, len(raw))
	for _, svc := range raw {
		key := svc.Name
		if svc.Config.IsPerUser {
			if base, ok := IsPerUserCandidate(svc.Name); ok {
				key = base
			}
		}
		svc.Instances = append([]string(nil), svc.Instances...)
		if existing, exists := out[key]; exists {
			existing.Instances = appendUnique(existing.Instances, svc.Name)
			// Keep a deterministic representative configuration. Prefer a
			// service whose live name is already the template/base key.
			if existing.Name != key && svc.Name == key {
				svc.Instances = appendUnique(svc.Instances, existing.Instances...)
				svc.Name = key
				out[key] = svc
			} else {
				out[key] = existing
			}
			continue
		}
		if len(svc.Instances) == 0 {
			svc.Instances = []string{svc.Name}
		}
		out[key] = svc
	}
	for key, svc := range out {
		if svc.Name == "" {
			svc.Name = key
		}
		if len(svc.Instances) == 0 {
			svc.Instances = []string{svc.Name}
		}
		out[key] = svc
	}
	return out
}

func appendUnique(dst []string, values ...string) []string {
	seen := make(map[string]struct{}, len(dst)+len(values))
	for _, v := range dst {
		seen[v] = struct{}{}
	}
	for _, v := range values {
		if _, ok := seen[v]; ok || v == "" {
			continue
		}
		seen[v] = struct{}{}
		dst = append(dst, v)
	}
	sort.Strings(dst)
	return dst
}

func ApplyStartup(service LiveService, target string) error {
	if err := ValidateStartType(target); err != nil {
		return err
	}
	names := appendUnique(nil, service.Instances...)
	if len(names) == 0 {
		names = []string{service.Name}
	}
	// Apply the active instances first. For a normalized per-user service,
	// main_api.go appends the base/template target as required by the spec.
	return SetServiceStartup(names, service.Name, service.Config.IsPerUser, target)
}

func ApplyBatch(entries []DiffEntry) []RemediationResult {
	results := make([]RemediationResult, 0, len(entries))
	for _, entry := range entries {
		result := RemediationResult{ServiceName: entry.Name}
		if entry.Target == nil || entry.Live == nil {
			result.Err = errors.New("invalid batch entry")
			results = append(results, result)
			continue
		}
		result.TargetState = entry.Target.StartType
		if entry.Target.StartType == "AutomaticDelayed" {
			result.TargetState = "AutomaticDelayed"
		}
		result.Err = ApplyStartup(*entry.Live, entry.Target.StartType)
		results = append(results, result)
	}
	return results
}

func AppendLiveOnly(snapshot Snapshot, live map[string]LiveService, names []string, now time.Time) Snapshot {
	merged := Snapshot{
		SchemaVersion: SchemaVersion,
		ExportedAt:    now.UTC().Format(time.RFC3339),
		SystemInfo:    snapshot.SystemInfo,
		Services:      make(map[string]ServiceConfig, len(snapshot.Services)+len(names)),
	}
	for name, cfg := range snapshot.Services {
		merged.Services[name] = cfg
	}
	for _, name := range names {
		if svc, ok := live[name]; ok {
			merged.Services[name] = svc.Config
		}
	}
	return merged
}

func SortedServiceNames(services map[string]ServiceConfig) []string {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func FileName(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Base(path)
}
