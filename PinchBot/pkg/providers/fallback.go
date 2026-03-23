package providers

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// FallbackChain orchestrates model fallback across multiple candidates.
type FallbackChain struct {
	cooldown *CooldownTracker
}

// FallbackCandidate represents one model/provider to try.
type FallbackCandidate struct {
	Provider string
	Model    string
}

// FallbackResult contains the successful response and metadata about all attempts.
type FallbackResult struct {
	Response *LLMResponse
	Provider string
	Model    string
	Attempts []FallbackAttempt
}

// FallbackAttempt records one attempt in the fallback chain.
type FallbackAttempt struct {
	Provider string
	Model    string
	Error    error
	Reason   FailoverReason
	Duration time.Duration
	Skipped  bool // true if skipped due to cooldown
	Layer    string
	Attempt  int
}

const (
	maxAttemptsPerCandidate = 2
	retryBackoffBase        = 200 * time.Millisecond
)

var multiKeySuffixPattern = regexp.MustCompile(`__key_\d+$`)

type layeredCandidate struct {
	candidate FallbackCandidate
	layer     string
}

// NewFallbackChain creates a new fallback chain with the given cooldown tracker.
func NewFallbackChain(cooldown *CooldownTracker) *FallbackChain {
	return &FallbackChain{cooldown: cooldown}
}

// ResolveCandidates parses model config into a deduplicated candidate list.
func ResolveCandidates(cfg ModelConfig, defaultProvider string) []FallbackCandidate {
	return ResolveCandidatesWithLookup(cfg, defaultProvider, nil)
}

func ResolveCandidatesWithLookup(
	cfg ModelConfig,
	defaultProvider string,
	lookup func(raw string) (resolved string, ok bool),
) []FallbackCandidate {
	seen := make(map[string]bool)
	var candidates []FallbackCandidate

	addCandidate := func(raw string) {
		candidateRaw := strings.TrimSpace(raw)
		if lookup != nil {
			if resolved, ok := lookup(candidateRaw); ok {
				candidateRaw = resolved
			}
		}

		ref := ParseModelRef(candidateRaw, defaultProvider)
		if ref == nil {
			return
		}
		key := ModelKey(ref.Provider, ref.Model)
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, FallbackCandidate{
			Provider: ref.Provider,
			Model:    ref.Model,
		})
	}

	// Primary first.
	addCandidate(cfg.Primary)

	// Then fallbacks.
	for _, fb := range cfg.Fallbacks {
		addCandidate(fb)
	}

	return candidates
}

// Execute runs the fallback chain for text/chat requests.
// It tries each candidate in order, respecting cooldowns and error classification.
//
// Behavior:
//   - Candidates are reordered into layers:
//   - same model retry (attempt 2 on current candidate)
//   - same provider/model-family (switch key/profile)
//   - same provider (switch model)
//   - cross provider (switch model/provider)
//   - Candidates in cooldown are skipped (logged as skipped attempt).
//   - context.Canceled aborts immediately (user abort, no fallback).
//   - Unknown/unclassified errors abort immediately (no fallback).
//   - Non-retriable errors (format) abort immediately.
//   - Retriable errors retry once on current candidate, then fallback.
//   - Success marks provider as good (resets cooldown).
//   - If all fail, returns aggregate error with all attempts.
func (fc *FallbackChain) Execute(
	ctx context.Context,
	candidates []FallbackCandidate,
	run func(ctx context.Context, provider, model string) (*LLMResponse, error),
) (*FallbackResult, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("fallback: no candidates configured")
	}

	layered := buildLayeredCandidates(candidates)
	result := &FallbackResult{
		Attempts: make([]FallbackAttempt, 0, len(layered)*maxAttemptsPerCandidate),
	}

	for _, candidate := range layered {
		// Check context before each attempt.
		if ctx.Err() == context.Canceled {
			return nil, context.Canceled
		}

		// Check cooldown per provider/model key.
		cooldownKey := ModelKey(candidate.candidate.Provider, candidate.candidate.Model)
		if !fc.cooldown.IsAvailable(cooldownKey) {
			remaining := fc.cooldown.CooldownRemaining(cooldownKey)
			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: candidate.candidate.Provider,
				Model:    candidate.candidate.Model,
				Skipped:  true,
				Reason:   FailoverRateLimit,
				Layer:    candidate.layer,
				Error: fmt.Errorf(
					"%s in cooldown (%s remaining)",
					cooldownKey,
					remaining.Round(time.Second),
				),
			})
			continue
		}

		for attempt := 1; attempt <= maxAttemptsPerCandidate; attempt++ {
			start := time.Now()
			resp, err := run(ctx, candidate.candidate.Provider, candidate.candidate.Model)
			elapsed := time.Since(start)

			if err == nil {
				// Success.
				fc.cooldown.MarkSuccess(cooldownKey)
				result.Response = resp
				result.Provider = candidate.candidate.Provider
				result.Model = candidate.candidate.Model
				return result, nil
			}

			// Context cancellation: abort immediately, no fallback.
			if ctx.Err() == context.Canceled {
				result.Attempts = append(result.Attempts, FallbackAttempt{
					Provider: candidate.candidate.Provider,
					Model:    candidate.candidate.Model,
					Error:    err,
					Duration: elapsed,
					Layer:    candidate.layer,
					Attempt:  attempt,
				})
				return nil, context.Canceled
			}

			// Classify the error.
			failErr := ClassifyError(err, candidate.candidate.Provider, candidate.candidate.Model)
			if failErr == nil {
				// Unclassified errors are treated conservatively: no fallback.
				result.Attempts = append(result.Attempts, FallbackAttempt{
					Provider: candidate.candidate.Provider,
					Model:    candidate.candidate.Model,
					Error:    err,
					Duration: elapsed,
					Layer:    candidate.layer,
					Attempt:  attempt,
				})
				return nil, fmt.Errorf(
					"fallback: unclassified error from %s/%s: %w",
					candidate.candidate.Provider,
					candidate.candidate.Model,
					err,
				)
			}

			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: candidate.candidate.Provider,
				Model:    candidate.candidate.Model,
				Error:    failErr,
				Reason:   failErr.Reason,
				Duration: elapsed,
				Layer:    candidate.layer,
				Attempt:  attempt,
			})

			// Non-retriable error: abort immediately.
			if !failErr.IsRetriable() {
				return nil, failErr
			}

			// Retry once on the same candidate before moving to next layer/candidate.
			if attempt < maxAttemptsPerCandidate {
				backoff := retryBackoffDuration(failErr.Reason, attempt)
				if backoff > 0 {
					timer := time.NewTimer(backoff)
					select {
					case <-ctx.Done():
						timer.Stop()
						return nil, context.Canceled
					case <-timer.C:
					}
				}
				continue
			}

			// Candidate exhausted: mark failure and move to next candidate.
			fc.cooldown.MarkFailure(cooldownKey, failErr.Reason)
		}
	}

	// All candidates failed or were skipped.
	return nil, &FallbackExhaustedError{Attempts: result.Attempts}
}

// ExecuteImage runs the fallback chain for image/vision requests.
// Simpler than Execute: no cooldown checks (image endpoints have different rate limits).
// Image dimension/size errors abort immediately (non-retriable).
func (fc *FallbackChain) ExecuteImage(
	ctx context.Context,
	candidates []FallbackCandidate,
	run func(ctx context.Context, provider, model string) (*LLMResponse, error),
) (*FallbackResult, error) {
	if len(candidates) == 0 {
		return nil, fmt.Errorf("image fallback: no candidates configured")
	}

	result := &FallbackResult{
		Attempts: make([]FallbackAttempt, 0, len(candidates)),
	}

	for i, candidate := range candidates {
		if ctx.Err() == context.Canceled {
			return nil, context.Canceled
		}

		start := time.Now()
		resp, err := run(ctx, candidate.Provider, candidate.Model)
		elapsed := time.Since(start)

		if err == nil {
			result.Response = resp
			result.Provider = candidate.Provider
			result.Model = candidate.Model
			return result, nil
		}

		if ctx.Err() == context.Canceled {
			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Error:    err,
				Duration: elapsed,
			})
			return nil, context.Canceled
		}

		// Image dimension/size errors are non-retriable.
		errMsg := strings.ToLower(err.Error())
		if IsImageDimensionError(errMsg) || IsImageSizeError(errMsg) {
			result.Attempts = append(result.Attempts, FallbackAttempt{
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Error:    err,
				Reason:   FailoverFormat,
				Duration: elapsed,
			})
			return nil, &FailoverError{
				Reason:   FailoverFormat,
				Provider: candidate.Provider,
				Model:    candidate.Model,
				Wrapped:  err,
			}
		}

		// Any other error: record and try next.
		result.Attempts = append(result.Attempts, FallbackAttempt{
			Provider: candidate.Provider,
			Model:    candidate.Model,
			Error:    err,
			Duration: elapsed,
		})

		if i == len(candidates)-1 {
			return nil, &FallbackExhaustedError{Attempts: result.Attempts}
		}
	}

	return nil, &FallbackExhaustedError{Attempts: result.Attempts}
}

// FallbackExhaustedError indicates all fallback candidates were tried and failed.
type FallbackExhaustedError struct {
	Attempts []FallbackAttempt
}

func (e *FallbackExhaustedError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("fallback: all %d attempts failed:", len(e.Attempts)))
	for i, a := range e.Attempts {
		if a.Skipped {
			sb.WriteString(fmt.Sprintf(
				"\n  [%d] %s/%s: skipped (cooldown, layer=%s)",
				i+1, a.Provider, a.Model, a.Layer,
			))
		} else {
			sb.WriteString(fmt.Sprintf(
				"\n  [%d] %s/%s: %v (reason=%s, layer=%s, attempt=%d, %s)",
				i+1, a.Provider, a.Model, a.Error, a.Reason, a.Layer, a.Attempt, a.Duration.Round(time.Millisecond),
			))
		}
	}
	return sb.String()
}

func buildLayeredCandidates(candidates []FallbackCandidate) []layeredCandidate {
	if len(candidates) == 0 {
		return nil
	}

	anchor := candidates[0]
	out := make([]layeredCandidate, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))

	appendUnique := func(c FallbackCandidate, layer string) {
		key := ModelKey(c.Provider, c.Model)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, layeredCandidate{candidate: c, layer: layer})
	}

	// Layer 1: current model (first candidate) + immediate same-model retry.
	appendUnique(anchor, "same_model_retry")

	// Layer 2: same provider + same model family (typically key/profile rotation).
	for i := 1; i < len(candidates); i++ {
		c := candidates[i]
		if sameProvider(anchor.Provider, c.Provider) && sameModelFamily(anchor.Model, c.Model) {
			appendUnique(c, "same_model_switch_key")
		}
	}

	// Layer 3: same provider, different model.
	for i := 1; i < len(candidates); i++ {
		c := candidates[i]
		if sameProvider(anchor.Provider, c.Provider) && !sameModelFamily(anchor.Model, c.Model) {
			appendUnique(c, "same_provider_switch_model")
		}
	}

	// Layer 4: cross provider.
	for i := 1; i < len(candidates); i++ {
		c := candidates[i]
		if !sameProvider(anchor.Provider, c.Provider) {
			appendUnique(c, "cross_provider_switch_model")
		}
	}

	return out
}

func sameProvider(a, b string) bool {
	return NormalizeProvider(a) == NormalizeProvider(b)
}

func sameModelFamily(a, b string) bool {
	return normalizeModelFamily(a) == normalizeModelFamily(b)
}

func normalizeModelFamily(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	model = multiKeySuffixPattern.ReplaceAllString(model, "")
	return model
}

func retryBackoffDuration(reason FailoverReason, attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	switch reason {
	case FailoverTimeout, FailoverRateLimit, FailoverUnknown:
		return time.Duration(attempt) * retryBackoffBase
	default:
		return 0
	}
}
