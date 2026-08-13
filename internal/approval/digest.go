package approval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func DigestArguments(args json.RawMessage) string {
	trimmed := bytes.TrimSpace(args)
	if len(trimmed) == 0 {
		trimmed = []byte("{}")
	}

	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return hashBytes(trimmed)
	}

	canonical, err := json.Marshal(value)
	if err != nil {
		return hashBytes(trimmed)
	}

	return hashBytes(canonical)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
