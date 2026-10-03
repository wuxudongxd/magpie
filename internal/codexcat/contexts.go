package codexcat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/filememo"
)

// Codex caches the list Magpie serves, including overridden windows. Keep
// upstream windows by the identity carried in that list's ETag so removing
// an override restores the right originals, even offline or after another
// account was asked. Immutable files also keep concurrent processes from
// replacing each other's snapshots. Only native context fields are kept,
// not credentials or the rest of each entry.
func contextsPath(id string) string {
	return filepath.Join(appdir.Config(), "codex-context-windows", id+".json")
}

// RememberContexts captures the source windows before overriding them and
// returns their identity. An overridden list is served only after its
// originals are saved. A null restores a context_window the backend omitted.
func RememberContexts(entries []any) (string, error) {
	windows := map[string]json.RawMessage{}
	for _, e := range entries {
		m, _ := e.(map[string]any)
		slug, _ := m["slug"].(string)
		if slug == "" {
			continue
		}
		v, err := json.Marshal(m["context_window"])
		if err != nil {
			return "", err
		}
		windows[slug] = v
	}
	b, err := json.Marshal(windows)
	if err != nil {
		return "", err
	}
	id := hashTag(b)
	path := contextsPath(id)
	if saved, err := os.ReadFile(path); err == nil && string(saved) == string(b) {
		return id, nil
	}
	if err := edit.WriteAtomic(path, b); err != nil {
		return "", fmt.Errorf("save Codex context windows: %w", err)
	}
	return id, nil
}

func restoreContexts(entries map[string]map[string]any, etag string) error {
	id := contextID(etag)
	if id == "" {
		return fmt.Errorf("Codex model cache has no original context snapshot")
	}
	b, err := os.ReadFile(contextsPath(id))
	if err != nil {
		return fmt.Errorf("read Codex context windows: %w", err)
	}
	var was map[string]json.RawMessage
	if json.Unmarshal(b, &was) != nil || was == nil || hashTag(b) != id {
		return fmt.Errorf("Codex context snapshot %s is invalid", id)
	}
	for slug, m := range entries {
		v, ok := was[slug]
		if !ok {
			return fmt.Errorf("Codex context snapshot has no window for %s", slug)
		}
		var value any
		if err := json.Unmarshal(v, &value); err != nil {
			return err
		}
		if value == nil {
			delete(m, "context_window")
		} else {
			m["context_window"] = value
		}
	}
	return nil
}

const contextsMark = "ctx."

// ContextTag includes the per-model and provider-wide settings in Codex's
// refetch tag, and marks a list whose native windows were overridden.
func ContextTag(tag string, windows map[string]int) string {
	if len(windows) == 0 {
		return tag
	}
	b, _ := json.Marshal(windows)
	return contextsMark + hashTag([]byte(tag+string(b)))
}

// ContextsETag names both the settings and the source snapshot for a list
// with overrides. The policy tag stays first for Tagged and MarkedV1.
func ContextsETag(etag, tag, id string) string {
	if id != "" {
		tag += "." + id
	}
	return WithTag(etag, tag)
}

// ResponseETag keeps the source snapshot suffix Codex already cached when
// both the upstream ETag and Magpie's settings tag are unchanged. A new
// vendor list or a settings edit still makes Codex ask for models again.
func ResponseETag(etag, tag string) string {
	if strings.HasPrefix(strings.TrimPrefix(tag, v1Mark), contextsMark) {
		cached, _ := filememo.Read("codex context etag", cachePath(), func(b []byte) (string, error) {
			var cache struct {
				ETag string `json:"etag"`
			}
			err := json.Unmarshal(b, &cache)
			return cache.ETag, err
		})
		if id := contextID(cached); id != "" && cached == ContextsETag(etag, tag, id) {
			return cached
		}
	}
	return WithTag(etag, tag)
}

func cachePath() string {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return filepath.Join(home, "models_cache.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "models_cache.json")
}

func contextID(etag string) string {
	_, tag, ok := strings.Cut(etag, tagMark)
	if !ok {
		return ""
	}
	tag = strings.TrimPrefix(tag, v1Mark)
	parts := strings.Split(strings.TrimSuffix(tag, `"`), ".")
	if len(parts) != 3 || parts[0]+"." != contextsMark {
		return ""
	}
	id := parts[2]
	if len(id) != 12 || strings.IndexFunc(id, func(r rune) bool {
		return !strings.ContainsRune("0123456789abcdef", r)
	}) >= 0 {
		return ""
	}
	return id
}

// MarkedContexts reports whether native windows in a cache were overridden,
// with or without the independent multi-agent V1 policy marker.
func MarkedContexts(etag string) bool {
	return strings.Contains(etag, tagMark+contextsMark) || strings.Contains(etag, tagMark+v1Mark+contextsMark)
}
