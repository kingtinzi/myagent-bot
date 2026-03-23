// Command scan-extension-matrix prints a draft extension capability matrix from
// openclaw.plugin.json plus heuristic detection of OpenClawPluginApi calls in index.ts.
// Run from PinchBot module root: go run ./cmd/scan-extension-matrix -extensions ./extensions
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// openClawAPIHints are substrings searched in index.ts (heuristic; not a full parser).
var openClawAPIHints = []struct {
	Tag    string
	Needle string
}{
	{"registerTool", "registerTool"},
	{"registerHook", "registerHook"},
	{"registerHttpRoute", "registerHttpRoute"},
	{"registerChannel", "registerChannel"},
	{"registerGatewayMethod", "registerGatewayMethod"},
	{"registerCli", "registerCli"},
	{"registerService", "registerService"},
	{"registerProvider", "registerProvider"},
	{"registerSpeechProvider", "registerSpeechProvider"},
	{"registerMediaUnderstandingProvider", "registerMediaUnderstandingProvider"},
	{"registerImageGenerationProvider", "registerImageGenerationProvider"},
	{"registerWebSearchProvider", "registerWebSearchProvider"},
	{"registerContextEngine", "registerContextEngine"},
	{"registerCommand", "registerCommand"},
	{"onConversationBindingResolved", "onConversationBindingResolved"},
	{"api.on(", "api.on("},
}

type pluginManifest struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	ConfigSchema json.RawMessage `json:"configSchema"`
}

func main() {
	extRoot := flag.String("extensions", "", "directory containing extension folders (default: ./extensions)")
	flag.Parse()
	root := strings.TrimSpace(*extRoot)
	if root == "" {
		root = "extensions"
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan-extension-matrix: %v\n", err)
		os.Exit(1)
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		fmt.Fprintf(os.Stderr, "scan-extension-matrix: not a directory: %s\n", abs)
		os.Exit(1)
	}

	rows, err := scan(abs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan-extension-matrix: %v\n", err)
		os.Exit(1)
	}
	if len(rows) == 0 {
		fmt.Fprintf(os.Stderr, "scan-extension-matrix: no openclaw.plugin.json under %s\n", abs)
		os.Exit(1)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].dir < rows[j].dir })

	fmt.Println("# Extension matrix draft (auto-generated)")
	fmt.Println()
	fmt.Println("Copy into `docs/extensions/<id>-matrix.md` (see `docs/extensions/extension-matrix-template.md`).")
	fmt.Println("Heuristic `index.ts` detection may miss dynamic calls or false-positive on comments.")
	fmt.Println()
	fmt.Println("| Directory | id | name |")
	fmt.Println("|-----------|----|-----|")
	for _, r := range rows {
		fmt.Printf("| %s | %s | %s |\n", r.dir, r.id, r.name)
	}
	fmt.Println()
	for _, r := range rows {
		printDetail(abs, r)
	}
}

type row struct {
	dir         string
	id          string
	name        string
	description string
	schemaHint  string
	indexPath   string
	indexSrc    string
}

func scan(root string) ([]row, error) {
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []row
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		manPath := filepath.Join(root, e.Name(), "openclaw.plugin.json")
		if _, err := os.Stat(manPath); err != nil {
			continue
		}
		data, err := os.ReadFile(manPath)
		if err != nil {
			return nil, err
		}
		var m pluginManifest
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", manPath, err)
		}
		r := row{
			dir:         e.Name(),
			id:          strings.TrimSpace(m.ID),
			name:        strings.TrimSpace(m.Name),
			description: strings.TrimSpace(m.Description),
		}
		if len(m.ConfigSchema) > 0 {
			r.schemaHint = strings.TrimSpace(string(m.ConfigSchema))
			if len(r.schemaHint) > 200 {
				r.schemaHint = r.schemaHint[:200] + "…"
			}
		}
		idx := filepath.Join(root, e.Name(), "index.ts")
		if st, err := os.Stat(idx); err == nil && !st.IsDir() {
			r.indexPath = idx
			b, err := os.ReadFile(idx)
			if err != nil {
				return nil, err
			}
			r.indexSrc = string(b)
		}
		out = append(out, r)
	}
	return out, nil
}

func printDetail(root string, r row) {
	fmt.Printf("## %s (`%s`)\n\n", r.id, r.dir)
	fmt.Println("**扩展 id / 名称**：")
	fmt.Println("- id:", r.id)
	fmt.Println("- name:", r.name)
	if r.description != "" {
		fmt.Println("- description:", r.description)
	}
	fmt.Println()
	fmt.Println("### §1 `openclaw.plugin.json`（扫描）")
	fmt.Println("- **configSchema**（截断）：")
	if r.schemaHint == "" {
		fmt.Println("  - *(missing or empty in file)*")
	} else {
		fmt.Println("  ```json")
		fmt.Println(r.schemaHint)
		fmt.Println("  ```")
	}
	fmt.Println()
	fmt.Println("### §2 `register(api)` API 线索（`index.ts` 子串启发式）")
	if r.indexPath == "" {
		fmt.Println("- *(no index.ts found)*")
		fmt.Println()
		return
	}
	rel := r.indexPath
	if rp, err := filepath.Rel(root, r.indexPath); err == nil && !strings.HasPrefix(rp, "..") {
		rel = rp
	}
	fmt.Println("- **file**: `" + rel + "`")
	var hits []string
	for _, h := range openClawAPIHints {
		if strings.Contains(r.indexSrc, h.Needle) {
			hits = append(hits, h.Tag)
		}
	}
	if len(hits) == 0 {
		fmt.Println("- *(no known API needles matched)*")
	} else {
		for _, t := range hits {
			fmt.Println("- [ ] `" + t + "`")
		}
	}
	fmt.Println()
}
