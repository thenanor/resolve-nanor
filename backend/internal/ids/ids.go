package ids

import "github.com/google/uuid"

// New generates an id like "tkt_a1b2c3d4" — a prefix plus the first 8 hex
// characters of a random UUID, matching the NestJS reference implementation.
func New(prefix string) string {
	return prefix + "_" + uuid.NewString()[:8]
}
