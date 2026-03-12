package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readAdminUI(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("admin_index.html"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	return string(content)
}

func extractJSFunction(t *testing.T, ui, signature string) string {
	t.Helper()
	start := strings.Index(ui, signature)
	if start < 0 {
		t.Fatalf("signature %q not found", signature)
	}
	bodyStart := strings.Index(ui[start:], "{")
	if bodyStart < 0 {
		t.Fatalf("opening brace for %q not found", signature)
	}
	depth := 0
	for i := start + bodyStart; i < len(ui); i++ {
		switch ui[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return ui[start : i+1]
			}
		}
	}
	t.Fatalf("closing brace for %q not found", signature)
	return ""
}

func TestAdminUIProvidesPreviewAndValidationHooks(t *testing.T) {
	ui := readAdminUI(t)

	if !strings.Contains(ui, `id="runtimePreviewGrid"`) {
		t.Fatal("expected runtime config preview grid for admin validation")
	}
	if !strings.Contains(ui, `function refreshRuntimeConfigPreview()`) {
		t.Fatal("expected runtime preview refresh logic")
	}
	if !strings.Contains(ui, `function validateRoutes(value)`) || !strings.Contains(ui, `function validateModels(value, routeIds)`) {
		t.Fatal("expected structured runtime-config validators for routes and models")
	}
	if !strings.Contains(ui, `function validatePricingRules(value, knownModelIDs)`) || !strings.Contains(ui, `function validateAgreements(value)`) {
		t.Fatal("expected structured runtime-config validators for pricing rules and agreements")
	}
	if !strings.Contains(ui, `id="save" disabled`) {
		t.Fatal("expected save button to stay disabled until validation/auth allow it")
	}
	if !strings.Contains(ui, `Enabled model "${modelID}" must also include a pricing rule.`) {
		t.Fatal("expected runtime config validation to reject enabled models without pricing")
	}
	if !strings.Contains(ui, `Model "${id}" must include enabled.`) {
		t.Fatal("expected runtime config validation to require explicit enabled flags")
	}
	if !strings.Contains(ui, `Route "${publicModelID}" is duplicated.`) || !strings.Contains(ui, `Pricing rule "${modelID}" is duplicated.`) {
		t.Fatal("expected runtime config validation to reject duplicate route/model pricing identifiers")
	}
	if !strings.Contains(ui, `Fix validation errors before saving runtime config.`) {
		t.Fatal("expected save flow to guard against invalid runtime config payloads")
	}

	saveBody := extractJSFunction(t, ui, `async function saveRuntimeConfig()`)
	if !strings.Contains(saveBody, `const validation = refreshRuntimeConfigPreview();`) ||
		!strings.Contains(saveBody, `if (validation.hasError)`) ||
		!strings.Contains(saveBody, `validation.parsedValues.routesInput`) {
		t.Fatal("expected saveRuntimeConfig() to re-run validation and only submit validated parsed values")
	}

	routeBody := extractJSFunction(t, ui, `function validateRoutes(value)`)
	if !strings.Contains(routeBody, `readOptionalStringField(item.model_config, field, modelConfigLabel, errors)`) {
		t.Fatal("expected route validation to type-check optional model_config string fields")
	}
	if !strings.Contains(routeBody, `validateOptionalIntegerField(item.model_config, field, modelConfigLabel, errors)`) {
		t.Fatal("expected route validation to type-check optional model_config integer fields")
	}
	pricingBody := extractJSFunction(t, ui, `function validatePricingRules(value, knownModelIDs)`)
	if !strings.Contains(pricingBody, `must be a non-negative integer.`) || !strings.Contains(pricingBody, `isIntegerNumber(item[field])`) {
		t.Fatal("expected pricing validation to reject fractional values")
	}
	modelBody := extractJSFunction(t, ui, `function validateModels(value, routeIds)`)
	if !strings.Contains(modelBody, `readOptionalStringField(item, 'name',`) {
		t.Fatal("expected model validation to type-check optional display names")
	}
	refreshBody := extractJSFunction(t, ui, `function refreshRuntimeConfigPreview()`)
	if !strings.Contains(refreshBody, `validateEnabledModelPricingCoverage`) {
		t.Fatal("expected refreshRuntimeConfigPreview() to enforce enabled-model pricing coverage")
	}
}

func TestAdminUIExposesAccessibleStatusRegions(t *testing.T) {
	ui := readAdminUI(t)

	if !strings.Contains(ui, `id="authStatus" class="status-line" role="status" aria-live="polite"`) {
		t.Fatal("expected auth status line to be announced accessibly")
	}
	if !strings.Contains(ui, `id="configStatus" class="status-line" role="status" aria-live="polite"`) {
		t.Fatal("expected config status line to be announced accessibly")
	}
	if !strings.Contains(ui, `class="editor-status"`) {
		t.Fatal("expected per-editor validation status blocks")
	}
	if !strings.Contains(ui, `<label for="routesInput">Official Routes</label>`) {
		t.Fatal("expected visible labels for runtime config editors")
	}
	if !strings.Contains(ui, `id="routesInput" aria-describedby="routesInputHelp routesInputStatus"`) {
		t.Fatal("expected runtime config editors to expose accessible descriptions")
	}
	if !strings.Contains(ui, `id="routesInputStatus" role="status" aria-live="polite"`) {
		t.Fatal("expected editor validation status blocks to be announced accessibly")
	}
	refreshBody := extractJSFunction(t, ui, `function setEditorStatus(inputId, statusId, text, kind)`)
	if !strings.Contains(refreshBody, `input.setAttribute('aria-invalid', kind === 'error' ? 'true' : 'false');`) {
		t.Fatal("expected invalid runtime config editors to expose aria-invalid state")
	}
}

func TestAdminUIRequiresReadableAgreementMetadata(t *testing.T) {
	ui := readAdminUI(t)
	helperBody := extractJSFunction(t, ui, `function readRequiredStringField(item, field, label, errors)`)
	if !strings.Contains(helperBody, `typeof item[field] !== 'string'`) {
		t.Fatal("expected runtime config validators to reject non-string required fields")
	}
	agreementBody := extractJSFunction(t, ui, `function validateAgreements(value)`)

	if !strings.Contains(agreementBody, `readRequiredStringField(item, 'title'`) {
		t.Fatal("expected agreement validation to require readable titles")
	}
	if !strings.Contains(agreementBody, `readOptionalStringField(item, 'url'`) {
		t.Fatal("expected agreement validation to type-check optional document URLs")
	}
	if !strings.Contains(agreementBody, `url must start with http:// or https://.`) {
		t.Fatal("expected agreement validation to reject invalid document URLs")
	}
}
