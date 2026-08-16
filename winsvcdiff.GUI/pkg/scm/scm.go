// pkg/scm/scm.go
package scm

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// ServiceState represents a detached snapshot of a Windows Service.
type ServiceState struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`       // Running, Stopped, etc.
	Startup     string `json:"startup_type"` // Disabled, Manual, Automatic, AutomaticDelayed
	ExePath     string `json:"exe_path"`
}

// GetAllServices enumerates all services in the SCM database.
// It opens and closes individual service handles sequentially to prevent
// handle exhaustion on systems with thousands of registered services.
func GetAllServices() (map[string]ServiceState, error) {
	// Connect requires administrative privileges for full inspection.
	m, err := mgr.Connect()
	if err != nil {
		return nil, fmt.Errorf("SCM connection failed: %w", err)
	}
	defer m.Disconnect()

	names, err := m.ListServices()
	if err != nil {
		return nil, fmt.Errorf("failed to enumerate services: %w", err)
	}

	services := make(map[string]ServiceState, len(names))
	for _, name := range names {
		// Isolated extraction to guarantee handle cleanup via defer per-iteration.
		state, err := extractServiceDef(m, name)
		if err != nil {
			// Some services (e.g., driver-level) might deny access even to Admin.
			// Log silently and skip to maintain operation continuity.
			continue
		}
		services[name] = state
	}

	return services, nil
}

// extractServiceDef handles the discrete lifecycle of querying a single service.
func extractServiceDef(m *mgr.Mgr, name string) (ServiceState, error) {
	s, err := m.OpenService(name)
	if err != nil {
		return ServiceState{}, err
	}
	defer s.Close()

	// Query runtime status (Running, Stopped, Paused, etc.)
	status, err := s.Query()
	if err != nil {
		return ServiceState{}, err
	}

	// Query configuration (Startup type, Binary path, Display Name)
	cfg, err := s.Config()
	if err != nil {
		return ServiceState{}, err
	}

	return ServiceState{
		Name:        name,
		DisplayName: cfg.DisplayName,
		Status:      mapSvcState(status.State),
		Startup:     mapStartupType(cfg.StartType, cfg.DelayedAutoStart),
		ExePath:     cfg.BinaryPathName,
	}, nil
}

// SetServiceStartup mutates the target service in the SCM.
func SetServiceStartup(name string, startup string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("SCM connect error: %w", err)
	}
	defer m.Disconnect()

	// SERVICE_CHANGE_CONFIG permission is required.
	s, err := m.OpenService(name)
	if err != nil {
		return fmt.Errorf("failed to open service %q: %w", name, err)
	}
	defer s.Close()

	cfg, err := s.Config()
	if err != nil {
		return fmt.Errorf("failed to read config for %q: %w", name, err)
	}

	startType, delayed, err := parseStartupType(startup)
	if err != nil {
		return err
	}

	// Only mutate if there's a difference to avoid unnecessary registry writes.
	if cfg.StartType == startType && cfg.DelayedAutoStart == delayed {
		return nil
	}

	cfg.StartType = startType
	cfg.DelayedAutoStart = delayed

	// UpdateConfig applies the struct via ChangeServiceConfigW.
	// DelayedAutoStart is applied via ChangeServiceConfig2W atomically within this call.
	if err := s.UpdateConfig(cfg); err != nil {
		return fmt.Errorf("failed to update config for %q: %w", name, err)
	}

	return nil
}

func mapSvcState(state svc.State) string {
	switch state {
	case svc.Stopped:
		return "Stopped"
	case svc.StartPending:
		return "StartPending"
	case svc.StopPending:
		return "StopPending"
	case svc.Running:
		return "Running"
	case svc.ContinuePending:
		return "ContinuePending"
	case svc.PausePending:
		return "PausePending"
	case svc.Paused:
		return "Paused"
	default:
		return "Unknown"
	}
}

func mapStartupType(startType uint32, delayed bool) string {
	switch startType {
	case windows.SERVICE_BOOT_START:
		return "Boot"
	case windows.SERVICE_SYSTEM_START:
		return "System"
	case windows.SERVICE_AUTO_START:
		if delayed {
			return "AutomaticDelayed"
		}
		return "Automatic"
	case windows.SERVICE_DEMAND_START:
		return "Manual"
	case windows.SERVICE_DISABLED:
		return "Disabled"
	default:
		return "Unknown"
	}
}

func parseStartupType(val string) (uint32, bool, error) {
	switch strings.ToLower(val) {
	case "automatic":
		return windows.SERVICE_AUTO_START, false, nil
	case "automaticdelayed":
		return windows.SERVICE_AUTO_START, true, nil
	case "manual":
		return windows.SERVICE_DEMAND_START, false, nil
	case "disabled":
		return windows.SERVICE_DISABLED, false, nil
	default:
		return 0, false, errors.New("unsupported startup type requested")
	}
}