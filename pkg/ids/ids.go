// Package ids generates identifiers shared by agent and control center.
package ids

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"regexp"
	"time"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// NewV7 returns a random, time-ordered UUID (RFC 9562 version 7) in its
// canonical lower-case form. Node, command and database ids use it.
func NewV7() string {
	return newV7(time.Now())
}

func newV7(now time.Time) string {
	var b [16]byte
	var ms [8]byte
	binary.BigEndian.PutUint64(ms[:], uint64(now.UnixMilli()))
	copy(b[0:6], ms[2:8])
	_, _ = rand.Read(b[6:]) // never fails since Go 1.24
	b[6] = 0x70 | (b[6] & 0x0f)
	b[8] = 0x80 | (b[8] & 0x3f)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// IsUUID reports whether s is a UUID in canonical lower-case form.
func IsUUID(s string) bool {
	return uuidPattern.MatchString(s)
}
