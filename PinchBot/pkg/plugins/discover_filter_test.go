package plugins

import "testing"

func TestExcludePluginIDs(t *testing.T) {
	t.Parallel()
	in := []DiscoveredPlugin{
		{ID: "lobster", Root: "/a"},
		{ID: "llm-task", Root: "/b"},
		{ID: "Other", Root: "/c"},
	}
	out := excludePluginIDs(in, []string{"llm-task"})
	if len(out) != 2 {
		t.Fatalf("len=%d want 2", len(out))
	}
	for _, p := range out {
		if p.ID == "llm-task" {
			t.Fatal("llm-task should be excluded")
		}
	}
}
