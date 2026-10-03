package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/provider"
)

// Failed persistence must not produce a cache that cannot undo its
// override. Offline, an unrestorable cache must not become a new original.
func TestCodexNativeContextSnapshotErrors(t *testing.T) {
	for _, failure := range []string{"save", "restore"} {
		t.Run(failure, func(t *testing.T) {
			codexSignedIn(t)
			offline := false
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if offline {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.Write([]byte(`{"models":[{"slug":"gpt-6.1-sol","context_window":272000}]}`))
			}))
			defer up.Close()
			was := provider.CodexBase
			provider.CodexBase = up.URL + "/backend-api/codex"
			t.Cleanup(func() { provider.CodexBase = was })
			if err := provider.Save(provider.Provider{ID: "codex", Contexts: map[string]int{"gpt-6.1-sol": 600000}}); err != nil {
				t.Fatal(err)
			}
			request := func() *httptest.ResponseRecorder {
				rec := httptest.NewRecorder()
				New().Handler().ServeHTTP(rec, httptest.NewRequest("GET", CodexPath+"/models", nil))
				return rec
			}
			dir := filepath.Join(appdir.Config(), "codex-context-windows")
			if failure == "save" {
				if err := os.WriteFile(dir, []byte("blocks directory"), 0o600); err != nil {
					t.Fatal(err)
				}
				if rec := request(); rec.Code != http.StatusInternalServerError || rec.Header().Get("ETag") != "" {
					t.Fatalf("failed save: %d %s", rec.Code, rec.Body.String())
				}
				return
			}
			rec := request()
			if rec.Code != 200 {
				t.Fatalf("initial response: %d %s", rec.Code, rec.Body.String())
			}
			var cached map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &cached); err != nil {
				t.Fatal(err)
			}
			cached["etag"] = rec.Header().Get("ETag")
			b, err := json.Marshal(cached)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(os.Getenv("HOME"), ".codex", "models_cache.json"), b, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(dir); err != nil {
				t.Fatal(err)
			}
			offline = true
			for _, contexts := range []map[string]int{nil, {"gpt-6.1-sol": 400000}} {
				if err := provider.Save(provider.Provider{ID: "codex", Contexts: contexts}); err != nil {
					t.Fatal(err)
				}
				if rec := request(); rec.Code != http.StatusBadGateway || rec.Header().Get("ETag") != "" {
					t.Errorf("unrestorable cache: %d %s", rec.Code, rec.Body.String())
				}
				if _, err := os.Stat(dir); !os.IsNotExist(err) {
					t.Error("overridden cache was saved as a new original")
				}
			}
		})
	}
}
