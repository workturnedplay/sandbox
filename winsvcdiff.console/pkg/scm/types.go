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

package scm

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

const (
	// SERVICE_WIN32_MASK filters SCM enumeration exclusively to user-mode services,
	// mirroring services.msc and excluding kernel/file system drivers.
	SERVICE_WIN32_MASK = windows.SERVICE_WIN32_OWN_PROCESS | windows.SERVICE_WIN32_SHARE_PROCESS

	// SERVICE_CONFIG_DELAYED_AUTO_START_INFO levels for QueryServiceConfig2 / ChangeServiceConfig2
	SERVICE_CONFIG_DELAYED_AUTO_START_INFO = 3
	SERVICE_CONFIG_TRIGGER_SET             = 5
)

// SERVICE_DELAYED_AUTO_START_INFO matches the Win32 structure for delayed autostart queries.
type SERVICE_DELAYED_AUTO_START_INFO struct {
	FDelayedAutostart uint32
}

// SERVICE_TRIGGER_INFO matches the Win32 header structure for querying service trigger setups.
type SERVICE_TRIGGER_INFO struct {
	CTriggers uint32
	PTriggers unsafe.Pointer
}

// ServiceInfo holds internal normalized metadata queried directly from SCM.
type ServiceInfo struct {
	Name             string // Canonical Base Template Name (e.g., "BluetoothUserService")
	LiveInstanceName string // Actual SCM Instance Name (e.g., "BluetoothUserService_a1b2c")
	DisplayName      string
	StartType        string // "Disabled", "Manual", "Automatic", "AutomaticDelayed", "Boot", "System"
	DelayedStart     bool
	TriggerStart     bool
	IsPerUser        bool
	LiveStatus       string // "Running", "Stopped", "Paused", "Start Pending", "Stop Pending"
}
