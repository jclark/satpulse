package sbfbin

// CRC16 computes the SBF CRC-16/CCITT checksum.
func CRC16(data []byte) uint16 {
	return uint16(crcMSB(data, 16, 0x1021, 0, 0))
}

func crcMSB(data []byte, width int, poly, init, xorout uint64) uint64 {
	mask := uint64(1)<<width - 1
	top := uint64(1) << (width - 1)
	crc := init
	for _, b := range data {
		crc ^= uint64(b) << (width - 8)
		for range 8 {
			if crc&top != 0 {
				crc = crc<<1 ^ poly
			} else {
				crc <<= 1
			}
		}
		crc &= mask
	}
	return (crc ^ xorout) & mask
}
