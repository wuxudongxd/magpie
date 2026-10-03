package codexcat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
)

// A missing or corrupt source snapshot cannot be reconstructed from an
// overridden cache. Refuse it instead of accepting a new upstream original.
func TestCodexContextSnapshotInvalid(t *testing.T) {
	for _, damaged := range []string{"missing", "null", "{}", "invalid json"} {
		t.Run(damaged, func(t *testing.T) {
			home := v1Home(t, `{}`)
			entries := []any{map[string]any{"slug": "gpt-6.1-sol", "context_window": 272000}}
			id, err := RememberContexts(entries)
			if err != nil {
				t.Fatal(err)
			}
			b, err := json.Marshal(map[string]any{
				"etag":   ContextsETag(`W/"native"`, ContextTag("models", map[string]int{"gpt-6.1-sol": 600000}), id),
				"models": []any{map[string]any{"slug": "gpt-6.1-sol", "context_window": 600000}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, ".codex", "models_cache.json"), b, 0o600); err != nil {
				t.Fatal(err)
			}
			if damaged == "missing" {
				err = os.Remove(contextsPath(id))
			} else {
				err = os.WriteFile(contextsPath(id), []byte(damaged), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if got, err := CacheEntriesWithError(); err == nil || got != nil {
				t.Errorf("unrestored cache accepted as upstream: %v, %v", got, err)
			}
			if b, err := Catalog([]catalog.Model{{ID: "codex/gpt-6.1-sol", Context: 600000}}); err == nil || b != nil {
				t.Errorf("native catalog silently lost metadata: %s, %v", b, err)
			}
			if _, err := Catalog([]catalog.Model{{ID: "relay/model", Context: 65536}}); err != nil {
				t.Errorf("third-party catalog unnecessarily needs a native snapshot: %v", err)
			}
			// A fresh native response can repair a damaged source snapshot.
			if _, err := RememberContexts(entries); err != nil {
				t.Fatal(err)
			}
			if got := CacheEntries(); got["gpt-6.1-sol"]["context_window"] != float64(272000) {
				t.Errorf("fresh source did not repair the snapshot: %v", got)
			}
		})
	}
}

func TestCodexContextSnapshotWriteFailure(t *testing.T) {
	v1Home(t, `{}`)
	if err := os.MkdirAll(filepath.Dir(contextsPath("test")), 0o755); err != nil {
		t.Fatal(err)
	}
	entries := []any{map[string]any{"slug": "gpt-6.1-sol", "context_window": 272000}}
	id, err := RememberContexts(entries)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(contextsPath(id)); err != nil {
		t.Fatal(err)
	}
	// A directory at the destination deterministically prevents a write,
	// including when the test runs with elevated filesystem privileges.
	if err := os.Mkdir(contextsPath(id), 0o755); err != nil {
		t.Fatal(err)
	}
	if id, err := RememberContexts(entries); err == nil || id != "" {
		t.Errorf("failed save returned usable source identity %q, %v", id, err)
	}
}

func TestCodexContextSnapshotOmittedWindow(t *testing.T) {
	v1Home(t, `{}`)
	entries := []any{map[string]any{"slug": "future-native", "supports_search_tool": true}}
	id, err := RememberContexts(entries)
	if err != nil {
		t.Fatal(err)
	}
	for _, v1 := range []bool{false, true} {
		setV1(t, v1)
		tag := PolicyTag(ContextTag("models", map[string]int{"*": 600000}))
		etag := ContextsETag(`W/"upstream.ctx.not-a-marker"`, tag, id)
		entries := map[string]map[string]any{"future-native": {"context_window": 600000, "supports_search_tool": true}}
		if err := restoreContexts(entries, etag); err != nil {
			t.Fatal(err)
		}
		if _, ok := entries["future-native"]["context_window"]; ok || entries["future-native"]["supports_search_tool"] != true {
			t.Errorf("omitted window not restored: %v", entries)
		}
	}
}
