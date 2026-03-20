package selfcheck

import (
	"bytes"
	"crypto/sha512"
	"errors"
	"io"
	"os"
)

var (
	errMarkerNotFound   = errors.New("marker not found")
	errMarkerAmbiguous  = errors.New("marker found more than once")
	errLayoutMismatch   = errors.New("layout version mismatch")
	errChecksumMismatch = errors.New("checksum mismatch")
)

type scanResult struct {
	markerOffset   int
	checksumOffset int
}

func scanForMarker(buf []byte) (scanResult, error) {
	marker := embeddedBlock.Marker[:]

	first := bytes.Index(buf, marker)
	if first < 0 {
		return scanResult{}, errMarkerNotFound
	}

	second := bytes.Index(buf[first+1:], marker)
	if second >= 0 {
		return scanResult{}, errMarkerAmbiguous
	}

	checksumOffset := first + len(marker)

	if checksumOffset+len(embeddedBlock.Checksum) > len(buf) {
		return scanResult{}, errors.New("checksum out of bounds")
	}

	return scanResult{
		markerOffset:   first,
		checksumOffset: checksumOffset,
	}, nil
}

func computeHashWithZeroedChecksum(buf []byte, sr scanResult) [64]byte {
	tmp := make([]byte, len(buf))
	copy(tmp, buf)

	for i := 0; i < len(embeddedBlock.Checksum); i++ {
		tmp[sr.checksumOffset+i] = 0
	}

	sum := sha512.Sum512(tmp)
	return sum
}

func readFileFully(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(f)
}
