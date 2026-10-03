package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/codexcat"
	"github.com/yetone/magpie/internal/provider"
)

// If restoring native windows fails, Sync preserves the existing catalog
// instead of replacing the native prompts and tools with generic entries.
func TestCodexContextSyncKeepsCatalogOnError(t *testing.T) {
	home, _ := codexHome(t, `{"tokens":{"access_token":"x","id_token":"x.e30.x"}}`, "")
	if err := catalog.SaveLive("codex", provider.CodexBase, []catalog.Model{{ID: "gpt-6.1-sol", Context: 272000, Images: true}}); err != nil {
		t.Fatal(err)
	}
	if err := provider.Save(provider.Provider{ID: "codex", Models: []string{"gpt-6.1-sol"}, Contexts: map[string]int{"gpt-6.1-sol": 600000}}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".codex")
	if err := os.WriteFile(filepath.Join(dir, "models_cache.json"), []byte(`{"models":[{"slug":"gpt-6.1-sol","context_window":272000,"base_instructions":"native prompt","use_responses_lite":true}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cx := codex(home)
	if err := cx.Field("login").Set("api"); err != nil {
		t.Fatal(err)
	}
	if err := cx.Fields[0].Set("codex/gpt-6.1-sol"); err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(dir, "magpie-models.json")
	before, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(before, &list); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range list.Models {
		if m["slug"] == "codex/gpt-6.1-sol" {
			found = true
			if m["context_window"] != float64(600000) || m["base_instructions"] != "native prompt" || m["use_responses_lite"] != true {
				t.Fatalf("native catalog: %v", m)
			}
		}
	}
	if !found {
		t.Fatal("native model missing from catalog")
	}
	b, err := json.Marshal(map[string]any{
		"etag":   codexcat.ContextsETag(`W/"native"`, codexcat.ContextTag("models", map[string]int{"gpt-6.1-sol": 600000}), "0123456789ab"),
		"models": []any{map[string]any{"slug": "gpt-6.1-sol", "context_window": 600000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "models_cache.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cx.Sync(); err == nil {
		t.Fatal("unrestorable native cache silently replaced the catalog")
	}
	if after, err := os.ReadFile(catalogPath); err != nil || string(after) != string(before) {
		t.Fatalf("existing catalog was changed: %v", err)
	}
}
