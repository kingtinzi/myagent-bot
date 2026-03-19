package config

import (
	"slices"
	"testing"
)

func findModelByName(models []ModelConfig, name string) *ModelConfig {
	for i := range models {
		if models[i].ModelName == name {
			return &models[i]
		}
	}
	return nil
}

func TestMergeAPIKeys(t *testing.T) {
	got := MergeAPIKeys(" key1 ", []string{"", "key2", "key1", " key3 "})
	want := []string{"key1", "key2", "key3"}
	if !slices.Equal(got, want) {
		t.Fatalf("MergeAPIKeys() = %v, want %v", got, want)
	}
}

func TestExpandMultiKeyModels_SingleKey(t *testing.T) {
	in := []ModelConfig{
		{
			ModelName: "gpt-4",
			Model:     "openai/gpt-4o",
			APIKey:    "k1",
		},
	}

	out := ExpandMultiKeyModels(in)
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	if out[0].ModelName != "gpt-4" || out[0].APIKey != "k1" {
		t.Fatalf("out[0] = %+v, want model_name=gpt-4 api_key=k1", out[0])
	}
	if len(out[0].Fallbacks) != 0 {
		t.Fatalf("fallbacks = %v, want empty", out[0].Fallbacks)
	}
}

func TestExpandMultiKeyModels_APIKeysExpand(t *testing.T) {
	in := []ModelConfig{
		{
			ModelName: "glm-4.7",
			Model:     "zhipu/glm-4.7",
			APIKeys:   []string{"k1", "k2", "k3"},
		},
	}

	out := ExpandMultiKeyModels(in)
	if len(out) != 3 {
		t.Fatalf("len(out) = %d, want 3", len(out))
	}

	primary := findModelByName(out, "glm-4.7")
	if primary == nil {
		t.Fatal("missing primary entry glm-4.7")
	}
	if primary.APIKey != "k1" {
		t.Fatalf("primary api_key = %q, want k1", primary.APIKey)
	}
	if !slices.Equal(primary.Fallbacks, []string{"glm-4.7__key_1", "glm-4.7__key_2"}) {
		t.Fatalf("primary fallbacks = %v, want [glm-4.7__key_1 glm-4.7__key_2]", primary.Fallbacks)
	}

	k2 := findModelByName(out, "glm-4.7__key_1")
	if k2 == nil || k2.APIKey != "k2" {
		t.Fatalf("entry glm-4.7__key_1 = %+v, want api_key=k2", k2)
	}
	k3 := findModelByName(out, "glm-4.7__key_2")
	if k3 == nil || k3.APIKey != "k3" {
		t.Fatalf("entry glm-4.7__key_2 = %+v, want api_key=k3", k3)
	}
}

func TestExpandMultiKeyModels_PreserveExistingFallbacks(t *testing.T) {
	in := []ModelConfig{
		{
			ModelName: "gpt-4",
			Model:     "openai/gpt-4o",
			APIKeys:   []string{"k1", "k2"},
			Fallbacks: []string{"claude"},
		},
	}

	out := ExpandMultiKeyModels(in)
	primary := findModelByName(out, "gpt-4")
	if primary == nil {
		t.Fatal("missing primary entry gpt-4")
	}
	wantFallbacks := []string{"gpt-4__key_1", "claude"}
	if !slices.Equal(primary.Fallbacks, wantFallbacks) {
		t.Fatalf("fallbacks = %v, want %v", primary.Fallbacks, wantFallbacks)
	}
}

func TestExpandMultiKeyModels_DeduplicateKeys(t *testing.T) {
	in := []ModelConfig{
		{
			ModelName: "gpt-4",
			Model:     "openai/gpt-4o",
			APIKey:    "k1",
			APIKeys:   []string{"k1", "k2", "k1"},
		},
	}

	out := ExpandMultiKeyModels(in)
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	primary := findModelByName(out, "gpt-4")
	if primary == nil {
		t.Fatal("missing primary entry")
	}
	if primary.APIKey != "k1" {
		t.Fatalf("primary api_key = %q, want k1", primary.APIKey)
	}
	if !slices.Equal(primary.Fallbacks, []string{"gpt-4__key_1"}) {
		t.Fatalf("fallbacks = %v, want [gpt-4__key_1]", primary.Fallbacks)
	}
}
