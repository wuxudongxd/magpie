package codexcat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCodexContextCustomHome(t *testing.T) {
	v1Home(t, `{"models":[{"slug":"gpt-6.1-sol","context_window":272000}]}`)
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	id, err := RememberContexts([]any{map[string]any{"slug": "gpt-6.1-sol", "context_window": 300000}})
	if err != nil {
		t.Fatal(err)
	}
	tag := ContextTag("models", map[string]int{"gpt-6.1-sol": 600000})
	etag := ContextsETag(`W/"native"`, tag, id)
	b, err := json.Marshal(map[string]any{"etag": etag, "models": []any{map[string]any{"slug": "gpt-6.1-sol", "context_window": 600000}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := CacheEntries(); got["gpt-6.1-sol"]["context_window"] != float64(300000) {
		t.Errorf("custom home source: %v", got)
	}
	if got := ResponseETag(`W/"native"`, tag); got != etag {
		t.Errorf("custom home response ETag: %q, want %q", got, etag)
	}
}
