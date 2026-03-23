package plugins

import (
	"strings"
	"testing"
)

func TestValidateOpenClawManifest_LobsterShape(t *testing.T) {
	raw := `{
  "id": "lobster",
  "name": "Lobster",
  "configSchema": {
    "type": "object",
    "additionalProperties": false,
    "properties": {}
  }
}`
	id, err := ValidateOpenClawManifest([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "lobster" {
		t.Fatalf("id = %q", id)
	}
}

func TestValidateOpenClawManifest_MissingConfigSchema(t *testing.T) {
	raw := `{"id": "x"}`
	_, err := ValidateOpenClawManifest([]byte(raw))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "configSchema") {
		t.Fatalf("error: %v", err)
	}
}

func TestValidateOpenClawManifest_NullConfigSchema(t *testing.T) {
	raw := `{"id": "x", "configSchema": null}`
	_, err := ValidateOpenClawManifest([]byte(raw))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateOpenClawManifest_ConfigSchemaNotObject(t *testing.T) {
	raw := `{"id": "x", "configSchema": "string"}`
	_, err := ValidateOpenClawManifest([]byte(raw))
	if err == nil {
		t.Fatal("expected error")
	}
}
