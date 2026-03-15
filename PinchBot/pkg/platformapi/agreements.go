package platformapi

import (
	"sort"
	"strings"
)

var authAgreementKeys = map[string]struct{}{
	"user_terms":     {},
	"privacy_policy": {},
}

func FilterAuthAgreements(docs []AgreementDocument) []AgreementDocument {
	filtered := make([]AgreementDocument, 0, len(docs))
	for _, doc := range docs {
		key := strings.TrimSpace(doc.Key)
		if _, ok := authAgreementKeys[key]; !ok {
			continue
		}
		filtered = append(filtered, doc)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Key == filtered[j].Key {
			return filtered[i].Version < filtered[j].Version
		}
		return filtered[i].Key < filtered[j].Key
	})
	return filtered
}
