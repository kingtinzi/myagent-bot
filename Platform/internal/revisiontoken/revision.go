package revisiontoken

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

func ForPayload(payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:]) + `"`, nil
}

func Matches(headerValue, current string) bool {
	for _, candidate := range strings.Split(headerValue, ",") {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			continue
		}
		if trimmed == "*" || trimmed == current {
			return true
		}
	}
	return false
}
