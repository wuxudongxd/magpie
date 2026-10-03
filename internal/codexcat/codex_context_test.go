package codexcat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
)

// A catalog written for Codex's provider mode keeps the native tool and
// prompt metadata, but uses the effective per-model context from Magpie.
func TestCodexCatalogOwnContextWindows(t *testing.T) {
	for _, v1 := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "v1"}[v1], func(t *testing.T) {
			v1Home(t, `{"models":[
				{"slug":"gpt-6.1-sol","context_window":272000,"max_context_window":872000,
				 "base_instructions":"native prompt","input_modalities":["text","image"],
				 "multi_agent_version":"v2","supports_search_tool":true,"use_responses_lite":true},
				{"slug":"gpt-6-astra","context_window":272000}]}`)
			setV1(t, v1)
			for _, window := range []int{600000, 128000, 0} {
				var got struct {
					Models []map[string]any `json:"models"`
				}
				if err := json.Unmarshal(catalogJSON(t, []catalog.Model{
					{ID: "codex/gpt-6.1-sol", Name: "Sol", Context: window},
					{ID: "codex/gpt-6-astra", Name: "Astra"},
				}), &got); err != nil || len(got.Models) != 2 {
					t.Fatalf("%v: %+v", err, got)
				}
				want := window
				if want == 0 {
					want = 272000
				}
				sol := got.Models[0]
				if sol["context_window"] != float64(want) || got.Models[1]["context_window"] != float64(272000) {
					t.Errorf("window %d: %+v", window, got.Models)
				}
				version := "v2"
				if v1 {
					version = "v1"
				}
				if sol["base_instructions"] != "native prompt" || sol["max_context_window"] != float64(872000) ||
					sol["multi_agent_version"] != version || sol["supports_search_tool"] != true || sol["use_responses_lite"] != true ||
					len(sol["input_modalities"].([]any)) != 2 {
					t.Errorf("native capabilities changed: %v", sol)
				}
				if CacheEntries()["gpt-6.1-sol"]["context_window"] != float64(272000) {
					t.Fatal("rendering changed the source cache")
				}
			}
			if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".codex", "models_cache.json")); err != nil {
				t.Fatal(err)
			}
		})
	}
}
