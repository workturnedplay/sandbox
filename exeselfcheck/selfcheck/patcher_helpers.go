package selfcheck

import "encoding/binary"

func readLayoutVersion(buf []byte, sr scanResult) uint32 {
	off := sr.checksumOffset + len(embeddedBlock.Checksum)
	return binary.LittleEndian.Uint32(buf[off:])
}

func writeStateInitialized(buf []byte, sr scanResult) {
	off := sr.checksumOffset + len(embeddedBlock.Checksum) + 4
	binary.LittleEndian.PutUint32(buf[off:], 1)
}
