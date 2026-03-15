package runtimeconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"openclaw/platform/internal/revisiontoken"
	"openclaw/platform/internal/upstream"
)

func RevisionForState(state State) (string, error) {
	return revisiontoken.ForPayload(revisionComparableState(state))
}

func RevisionForRoutes(routes []upstream.OfficialRoute) (string, error) {
	return revisiontoken.ForPayload(revisionComparableRoutes(routes))
}

func revisionComparableState(state State) State {
	out := cloneState(state)
	out.OfficialRoutes = revisionComparableRoutes(out.OfficialRoutes)
	return out
}

func revisionComparableRoutes(routes []upstream.OfficialRoute) []upstream.OfficialRoute {
	items := append([]upstream.OfficialRoute(nil), routes...)
	for i := range items {
		items[i].ModelConfig.APIKey = revisionComparableSecret(items[i].ModelConfig.APIKey)
	}
	return items
}

func revisionComparableSecret(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(secret))
	return "__SECRET_SHA256__:" + hex.EncodeToString(sum[:])
}
