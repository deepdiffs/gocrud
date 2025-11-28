package handlers

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"
)

// deterministicID returns a stable ID derived from the provided parts. This is
// used to keep creates idempotent when the same logical datapoint arrives more
// than once (e.g., re-importing a workout for the same start time).
func deterministicID(parts ...string) string {
	h := sha1.New()
	for _, p := range parts {
		value := strings.TrimSpace(p)
		h.Write([]byte(value))
		h.Write([]byte("|"))
	}
	return hex.EncodeToString(h.Sum(nil))
}
