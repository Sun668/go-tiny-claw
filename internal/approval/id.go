package approval

import (
	"crypto/rand"
	"encoding/hex"
)

func NewRequestID() string {
	bytes := make([]byte, 16)

	if _, err := rand.Read(bytes); err != nil {
		return ""
	}

	return hex.EncodeToString(bytes)
}
