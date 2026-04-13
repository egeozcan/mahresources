package types

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

// NewUUIDv7 generates a UUID v7 per RFC 9562: 48-bit millisecond timestamp,
// 4-bit version (0b0111), 12-bit random, 2-bit variant (0b10), 62-bit random.
// No external dependencies.
func NewUUIDv7() string {
	ms := uint64(time.Now().UnixMilli())

	var buf [16]byte
	// Bytes 0-5: 48-bit big-endian timestamp
	binary.BigEndian.PutUint16(buf[0:2], uint16(ms>>32))
	binary.BigEndian.PutUint32(buf[2:6], uint32(ms))

	// Bytes 6-15: random
	if _, err := rand.Read(buf[6:]); err != nil {
		panic(fmt.Sprintf("uuid_v7: crypto/rand failed: %v", err))
	}

	// Version 7: high nibble of byte 6
	buf[6] = (buf[6] & 0x0f) | 0x70
	// Variant 10: high 2 bits of byte 8
	buf[8] = (buf[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(buf[0:4]),
		binary.BigEndian.Uint16(buf[4:6]),
		binary.BigEndian.Uint16(buf[6:8]),
		binary.BigEndian.Uint16(buf[8:10]),
		buf[10:16],
	)
}
