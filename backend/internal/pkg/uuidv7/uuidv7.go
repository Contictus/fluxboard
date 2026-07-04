// Package uuidv7 generates time-ordered UUIDv7 identifiers used as primary keys.
// Time-ordering keeps B-tree index inserts sequential (docs/07-DATABASE-SCHEMA.md
// §Conventions). RFC 9562 layout: 48-bit unix_ts_ms | ver(7) | rand_a | var | rand_b.
package uuidv7

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// UUID is a 128-bit identifier.
type UUID [16]byte

// New returns a fresh UUIDv7 seeded from the current wall-clock millisecond.
func New() UUID { return NewAt(time.Now()) }

// NewAt returns a UUIDv7 whose timestamp field is t (millisecond precision).
func NewAt(t time.Time) UUID {
	var u UUID
	// 48-bit big-endian unix milliseconds.
	ms := uint64(t.UnixMilli())
	u[0] = byte(ms >> 40)
	u[1] = byte(ms >> 32)
	u[2] = byte(ms >> 24)
	u[3] = byte(ms >> 16)
	u[4] = byte(ms >> 8)
	u[5] = byte(ms)
	// Random for the remaining 74 bits (versions/variant patched below).
	if _, err := rand.Read(u[6:]); err != nil {
		panic("uuidv7: crypto/rand failed: " + err.Error())
	}
	u[6] = (u[6] & 0x0f) | 0x70 // version 7
	u[8] = (u[8] & 0x3f) | 0x80 // RFC 4122 variant
	return u
}

// String renders canonical 8-4-4-4-12 hyphenated form.
func (u UUID) String() string {
	buf := make([]byte, 36)
	hex.Encode(buf[0:8], u[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], u[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], u[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], u[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], u[10:16])
	return string(buf)
}
