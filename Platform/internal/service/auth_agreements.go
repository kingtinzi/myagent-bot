package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

var signupAgreementKeys = map[string]struct{}{
	"user_terms":     {},
	"privacy_policy": {},
}

func (s *Service) ListAuthAgreements(ctx context.Context) []AgreementDocument {
	return filterSignupAgreementDocuments(s.ListAgreements(ctx))
}

func (s *Service) ValidateRequiredAuthAgreements(
	ctx context.Context,
	docs []AgreementDocument,
) ([]AgreementDocument, error) {
	required := s.ListAuthAgreements(ctx)
	if len(required) == 0 {
		return nil, nil
	}

	submitted := filterSignupAgreementDocuments(docs)
	requiredByKey := make(map[string]AgreementDocument, len(required))
	for _, doc := range required {
		requiredByKey[strings.TrimSpace(doc.Key)] = doc
	}

	submittedByKey := make(map[string]AgreementDocument, len(submitted))
	for _, doc := range submitted {
		key := strings.TrimSpace(doc.Key)
		version := strings.TrimSpace(doc.Version)
		if key == "" || version == "" {
			return nil, fmt.Errorf("%w: key and version are required", ErrInvalidAgreement)
		}
		expected, ok := requiredByKey[key]
		if !ok {
			return nil, fmt.Errorf("%w: agreement %s version %s is not a current signup agreement", ErrUnknownAgreement, key, version)
		}
		if _, exists := submittedByKey[key]; exists {
			return nil, fmt.Errorf("%w: duplicate signup agreement %s", ErrInvalidAgreement, key)
		}
		if !sameAgreementDocument(doc, expected) {
			return nil, fmt.Errorf("%w: %s::%s does not match current published content", ErrInvalidAgreement, key, strings.TrimSpace(expected.Version))
		}
		submittedByKey[key] = doc
	}

	validated := make([]AgreementDocument, 0, len(required))
	for _, expected := range required {
		key := strings.TrimSpace(expected.Key)
		if _, ok := submittedByKey[key]; !ok {
			return nil, fmt.Errorf("%w: agreement %s version %s must be accepted before signup", ErrInvalidAgreement, key, strings.TrimSpace(expected.Version))
		}
		validated = append(validated, expected)
	}
	return validated, nil
}

func filterSignupAgreementDocuments(docs []AgreementDocument) []AgreementDocument {
	filtered := make([]AgreementDocument, 0, len(docs))
	for _, doc := range docs {
		key := strings.TrimSpace(doc.Key)
		if _, ok := signupAgreementKeys[key]; !ok {
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

func sameAgreementDocument(left, right AgreementDocument) bool {
	return strings.TrimSpace(left.Key) == strings.TrimSpace(right.Key) &&
		strings.TrimSpace(left.Version) == strings.TrimSpace(right.Version) &&
		strings.TrimSpace(left.Title) == strings.TrimSpace(right.Title) &&
		strings.TrimSpace(left.Content) == strings.TrimSpace(right.Content) &&
		strings.TrimSpace(left.URL) == strings.TrimSpace(right.URL)
}
