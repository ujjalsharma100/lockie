package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"time"
)

// NewValueID returns a freshly-generated ValueID using a UUID v7
// (RFC 9562 §5.7): 48 bits of unix-millisecond timestamp, then 74
// bits of CSPRNG entropy, with the version-7 and variant-10 bits set
// per the spec. UUID v7 is monotonic by birth time, which keeps
// "newest alias first" listings cheap without reading the value.
func NewValueID() (ValueID, error) {
	return newValueID(rand.Reader, time.Now())
}

func newValueID(r io.Reader, now time.Time) (ValueID, error) {
	var b [16]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return "", fmt.Errorf("store: read random for value-id: %w", err)
	}
	// UnixMilli returns int64; pre-1970 timestamps would corrupt the
	// monotonic-ordering property the format relies on, so clamp
	// non-negative. Cap at the 48-bit field width (year ~10889)
	// rather than wrap. Bit-shifting an int64 in [0, 2^48) is well-
	// defined and avoids the int64→uint64 cast gosec G115 flags.
	const ms48Max int64 = 1<<48 - 1
	ms := now.UnixMilli()
	if ms < 0 {
		ms = 0
	}
	if ms > ms48Max {
		ms = ms48Max
	}
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	b[6] = (b[6] & 0x0F) | 0x70 // version 7
	b[8] = (b[8] & 0x3F) | 0x80 // variant 10xx

	var buf [36]byte
	hex.Encode(buf[0:8], b[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], b[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], b[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], b[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], b[10:16])
	return ValueID(string(buf[:])), nil
}
