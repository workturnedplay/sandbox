package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func findInstanceID(vid, pid string) (string, error) {
	out, err := exec.Command("pnputil", "/enum-devices", "/connected", "/class", "USB").Output()
	if err != nil {
		return "", fmt.Errorf("pnputil enum failed: %w", err)
	}

	// match e.g. USB\VID_1532&PID_0109\5&1e7d8db7&0&14 but NOT ones containing MI_
	pattern := fmt.Sprintf(`USB\\VID_%s&PID_%s\\[^&\s]+$`, regexp.QuoteMeta(vid), regexp.QuoteMeta(pid))
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("regexp compile failed: %w", err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Instance ID:") {
			instanceID := strings.TrimSpace(strings.TrimPrefix(line, "Instance ID:"))
      			fmt.Printf("DEBUG candidate: %q  matches: %v\n", instanceID, re.MatchString(instanceID))
            prefix := fmt.Sprintf(`USB\VID_%s&PID_%s\`, vid, pid)
        if strings.HasPrefix(instanceID, prefix) && !strings.Contains(instanceID, "MI_") {
            return instanceID, nil
        }
			// if re.MatchString(instanceID) {
				// return instanceID, nil
			// }
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scanning output failed: %w", err)
	}

	return "", fmt.Errorf("device VID_%s&PID_%s not found", vid, pid)
}

func restartDevice(instanceID string) error {
	out, err := exec.Command("pnputil", "/restart-device", instanceID).CombinedOutput()
	if err != nil {
		return fmt.Errorf("pnputil restart failed: %w\noutput: %s", err, out)
	}
	fmt.Printf("%s\n", bytes.TrimSpace(out))
	return nil
}

func main() {
	const vid = "1532"
	const pid = "0109"

	instanceID, err := findInstanceID(vid, pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found: %s\n", instanceID)

	if err := restartDevice(instanceID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}