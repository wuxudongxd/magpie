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

// A response with unchanged model metadata must keep the ETag of the
// overridden list Codex cached, rather than trigger a refresh every turn.
func TestCodexNativeContextResponseETag(t *testing.T) {
	codexSignedIn(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `W/"native"`)
		w.Write([]byte(`{"models":[{"slug":"gpt-6.1-sol","context_window":272000}]}`))
	}))
	defer up.Close()
	was := provider.CodexBase
	provider.CodexBase = up.URL + "/backend-api/codex"
	t.Cleanup(func() { provider.CodexBase = was })
	if err := provider.Save(provider.Provider{ID: "codex", Contexts: map[string]int{"gpt-6.1-sol": 600000}}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	New().Handler().ServeHTTP(rec, httptest.NewRequest("GET", CodexPath+"/models", nil))
	var list map[string]any
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &list) != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	etag := rec.Header().Get("ETag")
	list["etag"] = etag
	b, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("HOME"), ".codex", "models_cache.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	header := func(upstream string) string {
		h := http.Header{"X-Models-Etag": {upstream}}
		modelsEtag(h)
		return h.Get("X-Models-Etag")
	}
	if got := header(`W/"native"`); got != etag {
		t.Errorf("unchanged response ETag %q, cached list %q", got, etag)
	}
	if got := header(`W/"updated"`); got == etag {
		t.Error("a new upstream ETag did not invalidate the list")
	}
	if err := provider.Save(provider.Provider{ID: "codex"}); err != nil {
		t.Fatal(err)
	}
	if got := header(`W/"native"`); got == etag {
		t.Error("reset did not invalidate the list")
	}
}
