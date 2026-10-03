package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/codexcat"
	"github.com/yetone/magpie/internal/provider"
)

// The GUI's subscription context setting must reach Codex's native list,
// both online and when the backend's last list stands in. Other native
// metadata and third-party windows remain their own.
func TestCodexNativeContextWindows(t *testing.T) {
	for _, offline := range []bool{false, true} {
		name := "backend"
		if offline {
			name = "cache"
		}
		t.Run(name, func(t *testing.T) {
			codexSignedIn(t)
			const source = `{"models":[
				{"slug":"gpt-6.1-sol","context_window":272000,"max_context_window":872000,
				 "base_instructions":"native prompt","input_modalities":["text","image"],
				 "multi_agent_version":"v2","supports_search_tool":true,"use_responses_lite":true},
				{"slug":"gpt-6-astra","context_window":272000},
				{"slug":"future-native","context_window":128000}]}`
			var saved struct {
				Models []map[string]any `json:"models"`
			}
			if err := json.Unmarshal([]byte(source), &saved); err != nil {
				t.Fatal(err)
			}
			if err := catalog.SaveLive("codex", "https://chatgpt.com/backend-api/codex", []catalog.Model{
				{ID: "gpt-6.1-sol", Context: 272000}, {ID: "gpt-6-astra", Context: 272000},
			}); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(os.Getenv("HOME"), ".codex", "models_cache.json"), []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if offline {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("ETag", `W/"native"`)
				io.WriteString(w, source)
			}))
			defer up.Close()
			was := provider.CodexBase
			provider.CodexBase = up.URL + "/backend-api/codex"
			t.Cleanup(func() { provider.CodexBase = was })
			if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Chat: up.URL, Key: "test", Models: []string{"small"}, Contexts: map[string]int{"*": 65536}}); err != nil {
				t.Fatal(err)
			}

			var previous string
			for _, c := range []struct {
				name     string
				contexts map[string]int
				sol      int
				astra    int
				future   int
			}{
				{"default", nil, 272000, 272000, 128000},
				{"model", map[string]int{"gpt-6.1-sol": 600000}, 600000, 272000, 128000},
				{"provider", map[string]int{"*": 400000, "gpt-6.1-sol": 600000}, 600000, 400000, 400000},
				{"smaller", map[string]int{"gpt-6.1-sol": 128000}, 128000, 272000, 128000},
				{"reset", nil, 272000, 272000, 128000},
			} {
				if err := provider.Save(provider.Provider{ID: "codex", Contexts: c.contexts}); err != nil {
					t.Fatal(err)
				}
				rec := httptest.NewRecorder()
				req := httptest.NewRequest("GET", CodexPath+"/models", nil)
				req.Header.Set("Authorization", "Bearer chatgpt-token")
				New().Handler().ServeHTTP(rec, req)
				var got struct {
					Models []map[string]any `json:"models"`
				}
				if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
					t.Fatalf("%s: %d %s", c.name, rec.Code, rec.Body.String())
				}
				by := map[string]map[string]any{}
				for _, m := range got.Models {
					by[m["slug"].(string)] = m
				}
				for slug, want := range map[string]int{"gpt-6.1-sol": c.sol, "gpt-6-astra": c.astra, "future-native": c.future, "relay/small": 65536} {
					if by[slug]["context_window"] != float64(want) {
						t.Errorf("%s: %s context_window = %v, want %d", c.name, slug, by[slug]["context_window"], want)
					}
				}
				sol := by["gpt-6.1-sol"]
				if sol["base_instructions"] != "native prompt" || sol["max_context_window"] != float64(872000) ||
					sol["multi_agent_version"] != "v2" || sol["supports_search_tool"] != true || sol["use_responses_lite"] != true ||
					len(sol["input_modalities"].([]any)) != 2 {
					t.Errorf("%s: native capabilities changed: %v", c.name, sol)
				}
				etag := rec.Header().Get("ETag")
				if previous != "" && etag == previous {
					t.Errorf("%s: context setting did not change ETag %q", c.name, etag)
				}
				h := http.Header{"X-Models-Etag": {`W/"native"`}}
				modelsEtag(h)
				if !codexcat.Tagged(h.Get("X-Models-Etag"), provider.CodexListTag()) {
					t.Errorf("%s: response ETag does not include context setting", c.name)
				}
				previous = etag
				// Codex persists exactly the list it was handed. The next
				// request must undo a removed override even offline.
				b, err := json.Marshal(map[string]any{"etag": etag, "models": got.Models})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(os.Getenv("HOME"), ".codex", "models_cache.json"), b, 0o600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
