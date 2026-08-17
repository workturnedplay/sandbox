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

//go:generate go run github.com/akavel/rsrc@latest -manifest app_manifest.xml -o app_manifest.syso

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"win-svc-diff/internal/wtw"
)

type app struct {
	mu sync.RWMutex

	snapshot       wtw.Snapshot
	snapshotLoaded bool
	snapshotPath   string
	live           map[string]wtw.LiveService
}

func main() {
	// Elevation is deliberately the very first operation. No GUI, SCM handle,
	// common-control initialization, or partial window is created before UAC.
	if !wtw.IsElevated() {
		if err := wtw.RelaunchElevated(os.Args[1:]); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	var a app
	exitCode := wtw.RunUI(wtw.UIHandlers{
		OnReady: func(ui *wtw.UI) {
			ui.SetStatus("Loading live service state...")
			setBusyUI(ui, true)
			ui.SetActionEnabled("apply", false)
			ui.SetActionEnabled("append", false)
			go a.refreshAsync(ui)
		},
		OnAction: a.handleAction,
		OnTabChanged: func(tab int, ui *wtw.UI) {
			if ui.Busy() {
				ui.SetActionEnabled("apply", false)
				ui.SetActionEnabled("append", false)
				return
			}
			ui.SetActionEnabled("apply", tab == 0)
			ui.SetActionEnabled("append", tab == 1)
		},
		OnStartupChanged: a.handleStartupChange,
		OnClose:          func(ui *wtw.UI) {},
	})
	os.Exit(exitCode)
}

func setBusyUI(ui *wtw.UI, busy bool) {
	ui.SetBusy(busy)
	ui.SetListsEnabled(!busy)
	ui.SetActionEnabled("refresh", !busy)
	if busy {
		ui.SetActionEnabled("apply", false)
		ui.SetActionEnabled("append", false)
		return
	}
	switch ui.CurrentTab() {
	case 0:
		ui.SetActionEnabled("apply", true)
		ui.SetActionEnabled("append", false)
	case 1:
		ui.SetActionEnabled("apply", false)
		ui.SetActionEnabled("append", true)
	default:
		ui.SetActionEnabled("apply", false)
		ui.SetActionEnabled("append", false)
	}
}

func (a *app) handleAction(action string, ui *wtw.UI) {
	switch action {
	case "save_live":
		a.saveLive(ui)
	case "load_file":
		a.loadFile(ui)
	case "refresh":
		ui.SetStatus("Refreshing system state...")
		setBusyUI(ui, true)
		go a.refreshAsync(ui)
	case "apply":
		a.applySelected(ui)
	case "append":
		a.appendSelected(ui)
	}
}

func (a *app) refreshAsync(ui *wtw.UI) {
	live, err := wtw.EnumerateLiveServices()
	_ = ui.Post(func() {
		setBusyUI(ui, false)
		if err != nil {
			ui.SetStatus("SCM refresh failed")
			ui.ShowError("System State Refresh Failed", err)
			return
		}
		a.mu.Lock()
		a.live = live
		snapshotLoaded := a.snapshotLoaded
		snapshot := a.snapshot
		a.mu.Unlock()

		if !snapshotLoaded {
			snapshot = wtw.Snapshot{SchemaVersion: wtw.SchemaVersion, Services: map[string]wtw.ServiceConfig{}}
		}
		a.render(ui, snapshot, snapshotLoaded, live)
		ui.SetStatus(fmt.Sprintf("System state refreshed: %d services", len(live)))
	})
}

func (a *app) render(ui *wtw.UI, snapshot wtw.Snapshot, snapshotLoaded bool, live map[string]wtw.LiveService) {
	a.mu.RLock()
	snapshotPath := a.snapshotPath
	a.mu.RUnlock()

	diff := wtw.DiffSnapshots(snapshot, live)
	rows := [4][]wtw.UIRow{}
	rows[0] = makeRows(diff.Mismatched)
	rows[1] = makeRows(diff.LiveOnly)
	rows[2] = makeRows(diff.FileOnly)
	rows[3] = makeRows(diff.Matched)
	ui.SetRows(0, rows[0])
	ui.SetRows(1, rows[1])
	ui.SetRows(2, rows[2])
	ui.SetRows(3, rows[3])
	ui.SetTabCounts([4]int{len(rows[0]), len(rows[1]), len(rows[2]), len(rows[3])})

	if snapshotLoaded {
		ui.SetTarget(fmt.Sprintf("Target Snapshot: %s (Services in File: %d)", wtw.FileName(snapshotPath), len(snapshot.Services)))
	} else {
		ui.SetTarget(fmt.Sprintf("Target Snapshot: No file loaded (Live services: %d)", len(live)))
	}
}

func makeRows(entries []wtw.DiffEntry) []wtw.UIRow {
	rows := make([]wtw.UIRow, 0, len(entries))
	for _, entry := range entries {
		row := wtw.UIRow{Key: entry.Name}
		if entry.Live != nil {
			row.DisplayName = entry.Live.Config.DisplayName
			row.Status = entry.Live.CurrentStatus
			row.LiveStartup = entry.Live.Config.StartType
			row.Editable = true
		}
		if entry.Target != nil {
			row.Target = entry.Target.StartType
		}
		rows = append(rows, row)
	}
	return rows
}

func (a *app) saveLive(ui *wtw.UI) {
	a.mu.RLock()
	live := cloneLive(a.live)
	a.mu.RUnlock()
	if len(live) == 0 {
		ui.ShowError("Save Current State", "No live service state is loaded yet.")
		return
	}
	defaultName := fmt.Sprintf("Win11-Service-State-%s.json", time.Now().Format("20060102-150405"))
	path, err := ui.PromptSaveJSON(defaultName)
	if err != nil {
		ui.ShowError("Save Current State", err)
		return
	}
	if path == "" {
		return
	}
	snapshot := wtw.NewLiveSnapshot(live, wtw.SystemInfoNow(), time.Now())
	if err := wtw.SaveSnapshot(path, snapshot); err != nil {
		ui.ShowError("Save Current State", err)
		return
	}
	ui.SetStatus(fmt.Sprintf("Saved live state to %s", path))
}

func (a *app) loadFile(ui *wtw.UI) {
	path, err := ui.PromptOpenJSON()
	if err != nil {
		ui.ShowError("Load State File", err)
		return
	}
	if path == "" {
		return
	}
	snapshot, err := wtw.LoadSnapshot(path)
	if err != nil {
		ui.ShowError("Load State File", err)
		return
	}
	a.mu.Lock()
	a.snapshot = snapshot
	a.snapshotLoaded = true
	a.snapshotPath = path
	live := cloneLive(a.live)
	a.mu.Unlock()
	if len(live) == 0 {
		ui.SetStatus("Snapshot loaded; live state is still loading...")
		return
	}
	a.render(ui, snapshot, true, live)
	ui.SetStatus(fmt.Sprintf("Loaded %s", path))
}

func (a *app) handleStartupChange(tab int, serviceName, target string, ui *wtw.UI) {
	if tab != 0 && tab != 1 && tab != 3 {
		return
	}
	a.mu.RLock()
	live, ok := a.live[serviceName]
	a.mu.RUnlock()
	if !ok {
		ui.ShowError("Startup Type Change", fmt.Sprintf("Service %q is no longer present in the live SCM snapshot.", serviceName))
		setBusyUI(ui, false)
		return
	}
	setBusyUI(ui, true)
	ui.SetStatus(fmt.Sprintf("Applying %s to %s...", target, serviceName))
	go func() {
		err := wtw.ApplyStartup(live, target)
		refreshed, refreshErr := wtw.EnumerateLiveServices()
		_ = ui.Post(func() {
			setBusyUI(ui, false)
			if err != nil {
				ui.ShowError("Startup Type Change Failed", fmt.Sprintf("%s: %v", serviceName, err))
			}
			if refreshErr != nil {
				ui.ShowError("Post-change Refresh Failed", refreshErr)
				return
			}
			a.mu.Lock()
			a.live = refreshed
			snapshot := a.snapshot
			loaded := a.snapshotLoaded
			a.mu.Unlock()
			if !loaded {
				snapshot = wtw.Snapshot{SchemaVersion: wtw.SchemaVersion, Services: map[string]wtw.ServiceConfig{}}
			}
			a.render(ui, snapshot, loaded, refreshed)
			if err == nil {
				ui.SetStatus(fmt.Sprintf("Updated %s -> %s", serviceName, target))
			} else {
				ui.SetStatus("Startup type change failed; live state refreshed")
			}
		})
	}()
}

func (a *app) applySelected(ui *wtw.UI) {
	if !a.snapshotLoaded {
		ui.ShowError("Apply File State", "Load a target snapshot before applying file state.")
		return
	}
	keys := ui.CheckedKeys(0)
	if len(keys) == 0 {
		ui.ShowError("Apply File State", "No services are selected in Tab 1 (Mismatched).")
		return
	}
	a.mu.RLock()
	live := cloneLive(a.live)
	snapshot := a.snapshot
	a.mu.RUnlock()
	diff := wtw.DiffSnapshots(snapshot, live)
	entries := entriesByName(diff.Mismatched, keys)
	if len(entries) == 0 {
		ui.ShowError("Apply File State", "The selected services are no longer mismatched; refresh and try again.")
		return
	}
	setBusyUI(ui, true)
	ui.SetStatus(fmt.Sprintf("Applying file state to %d services...", len(entries)))
	go func() {
		results := wtw.ApplyBatch(entries)
		refreshed, refreshErr := wtw.EnumerateLiveServices()
		_ = ui.Post(func() {
			setBusyUI(ui, false)
			if refreshErr != nil {
				ui.ShowAuditReport(results)
				ui.ShowError("Post-batch Refresh Failed", refreshErr)
				return
			}
			a.mu.Lock()
			a.live = refreshed
			snapshot := a.snapshot
			a.mu.Unlock()
			a.render(ui, snapshot, true, refreshed)
			ui.ShowAuditReport(results)
			ui.SetStatus("Batch remediation completed; results refreshed")
		})
	}()
}

func (a *app) appendSelected(ui *wtw.UI) {
	if !a.snapshotLoaded {
		ui.ShowError("Append Live Only", "Load a target snapshot before appending Live Only services.")
		return
	}
	keys := ui.CheckedKeys(1)
	if len(keys) == 0 {
		ui.ShowError("Append Live Only", "No services are selected in Tab 2 (Live Only).")
		return
	}
	a.mu.RLock()
	snapshot := a.snapshot
	live := cloneLive(a.live)
	a.mu.RUnlock()
	diff := wtw.DiffSnapshots(snapshot, live)
	selected := entriesByName(diff.LiveOnly, keys)
	selectedNames := make([]string, 0, len(selected))
	for _, e := range selected {
		selectedNames = append(selectedNames, e.Name)
	}
	if len(selectedNames) == 0 {
		ui.ShowError("Append Live Only", "The selected services are no longer Live Only; refresh and try again.")
		return
	}
	base := filepath.Base(a.snapshotPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	defaultName := stem + "-merged.json"
	path, err := ui.PromptSaveJSON(defaultName)
	if err != nil {
		ui.ShowError("Append Live Only", err)
		return
	}
	if path == "" {
		return
	}
	merged := wtw.AppendLiveOnly(snapshot, live, selectedNames, time.Now())
	if err := wtw.SaveSnapshot(path, merged); err != nil {
		ui.ShowError("Append Live Only", err)
		return
	}
	a.mu.Lock()
	a.snapshot = merged
	a.snapshotPath = path
	a.snapshotLoaded = true
	a.mu.Unlock()
	a.render(ui, merged, true, live)
	ui.SetStatus(fmt.Sprintf("Saved merged snapshot to %s", path))
}

func entriesByName(entries []wtw.DiffEntry, keys []string) []wtw.DiffEntry {
	wanted := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		wanted[k] = struct{}{}
	}
	out := make([]wtw.DiffEntry, 0, len(keys))
	for _, entry := range entries {
		if _, ok := wanted[entry.Name]; ok {
			out = append(out, entry)
		}
	}
	return out
}

func cloneLive(in map[string]wtw.LiveService) map[string]wtw.LiveService {
	if in == nil {
		return nil
	}
	out := make(map[string]wtw.LiveService, len(in))
	for k, v := range in {
		v.Instances = append([]string(nil), v.Instances...)
		out[k] = v
	}
	return out
}
