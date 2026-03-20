package selfcheck

import (
	"errors"
	"os"
)

func PatchExecutable(path string) error {
	buf, err := readFileFully(path)
	if err != nil {
		showDialog("Patching failed",
			"Unable to read target executable.",
			false)
		return err
	}

	sr, err := scanForMarker(buf)
	if err != nil {
		showDialog("Patching failed",
			"Marker not found or ambiguous.",
			false)
		return err
	}

	if embeddedBlock.LayoutVersion != readLayoutVersion(buf, sr) {
		showDialog("Patching failed",
			"Layout version mismatch.",
			false)
		return errLayoutMismatch
	}

	sum := computeHashWithZeroedChecksum(buf, sr)

	copy(buf[sr.checksumOffset:], sum[:])
	writeStateInitialized(buf, sr)

	//err = os.WriteFile(path, buf, 0)
	info, _ := os.Stat(path)
mode := os.FileMode(0666)
if info != nil {
	mode = info.Mode()
}
err = os.WriteFile(path, buf, mode)

	if err != nil {
		showDialog("Patching failed",
			"Unable to write patched executable.",
			false)
		return err
	}

	// Deterministic re-verify
	buf2, err := readFileFully(path)
	if err != nil {
		return err
	}

	sr2, err := scanForMarker(buf2)
	if err != nil {
		return err
	}

	sum2 := computeHashWithZeroedChecksum(buf2, sr2)
	if sum2 != sum {
		showDialog("Patching failed",
			"Post-patch verification failed.",
			false)
		return errors.New("nondeterministic patch")
	}

	showDialog("Patching successful",
		"Executable successfully patched.",
		false)

	return nil
}
