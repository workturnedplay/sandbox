package selfcheck

import (
	"errors"
)

var lastState State = StateUnknown

func VerifyAtStartup() (State, error) {
	path, err := getOwnExecutablePath()
	if err != nil {
		lastState = StateUnknown
		showDialog("Integrity check failed",
			"Unable to determine executable path.\n\nContinue anyway?",
			true)
		return lastState, err
	}

	buf, err := readFileFully(path)
	if err != nil {
		lastState = StateUnknown
		showDialog("Integrity check failed",
			"Unable to read executable.\n\nContinue anyway?",
			true)
		return lastState, err
	}

	sr, err := scanForMarker(buf)
	if err != nil {
		lastState = StateTainted
		showDialog("Integrity check failed",
			"Executable integrity data is invalid.\n\nContinue anyway?",
			true)
		return lastState, err
	}

	sum := computeHashWithZeroedChecksum(buf, sr)

	switch embeddedBlock.State {
	case 0:
		lastState = StateUninitialized
		showDialog("Executable not finalized",
			"This executable has not been patched yet.\n\nContinue anyway?",
			true)
		return lastState, errors.New("executable not patched")

	case 1:
		if sum != embeddedBlock.Checksum {
			lastState = StateTainted
			showDialog("Integrity check failed",
				"Executable contents have been modified.\n\nContinue anyway?",
				true)
			return lastState, errChecksumMismatch
		}

		lastState = StateVerified
		return lastState, nil

	default:
		lastState = StateTainted
		showDialog("Integrity check failed",
			"Executable state is invalid.\n\nContinue anyway?",
			true)
		return lastState, errors.New("invalid state field")
	}
}

func StateValue() State {
	return lastState
}
