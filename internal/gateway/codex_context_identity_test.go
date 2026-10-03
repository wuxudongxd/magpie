package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// A response's originals belong to that exact list. An older local cache,
// or another account returning the same upstream ETag, must not replace
// them before an offline reset.
func TestCodexNativeContextCacheIdentity(t *testing.T) {
	codexSignedIn(t)
	cache := filepath.Join(os.Getenv("HOME"), ".codex", "models_cache.json")
	if err := os.WriteFile(cache, []byte(`{"models":[{"slug":"gpt-6.1-sol","context_window":272000}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	offline := false
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if offline {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		window := 300000
		if r.Header.Get("chatgpt-account-id") == "account-b" {
			window = 200000
		}
		w.Header().Set("ETag", `W/"same-upstream"`)
		json.NewEncoder(w).Encode(map[string]any{"models": []any{map[string]any{"slug": "gpt-6.1-sol", "context_window": window}}})
	}))
	defer up.Close()
	was := provider.CodexBase
	provider.CodexBase = up.URL + "/backend-api/codex"
	t.Cleanup(func() { provider.CodexBase = was })
	if err := provider.Save(provider.Provider{ID: "codex", Contexts: map[string]int{"gpt-6.1-sol": 600000}}); err != nil {
		t.Fatal(err)
	}
	list := func(account string) (string, []map[string]any) {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", CodexPath+"/models", nil)
		req.Header.Set("Authorization", "Bearer test")
		req.Header.Set("chatgpt-account-id", account)
		New().Handler().ServeHTTP(rec, req)
		var got struct {
			Models []map[string]any `json:"models"`
		}
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
		return rec.Header().Get("ETag"), got.Models
	}
	type savedList struct {
		etag   string
		models []map[string]any
	}
	lists := map[string]savedList{}
	for _, account := range []string{"account-a", "account-b"} {
		etag, models := list(account)
		lists[account] = savedList{etag, models}
	}
	if lists["account-a"].etag == lists["account-b"].etag {
		t.Error("different accounts share the context snapshot identity")
	}
	if err := provider.Save(provider.Provider{ID: "codex"}); err != nil {
		t.Fatal(err)
	}
	offline = true
	for account, want := range map[string]int{"account-a": 300000, "account-b": 200000} {
		saved := lists[account]
		b, err := json.Marshal(map[string]any{"etag": saved.etag, "models": saved.models})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cache, b, 0o600); err != nil {
			t.Fatal(err)
		}
		_, models := list(account)
		for _, m := range models {
			if m["slug"] == "gpt-6.1-sol" && m["context_window"] != float64(want) {
				t.Errorf("%s reset offline: %v, want %d", account, m["context_window"], want)
			}
		}
	}
}
