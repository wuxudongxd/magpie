package provider

import (
	"testing"

	"github.com/yetone/magpie/internal/codexcat"
)

// Native subscription entries are excluded from CodexListed; their
// explicit context settings still need to invalidate Codex's cached list.
func TestCodexListTagContextWindows(t *testing.T) {
	signIn(t)
	base := CodexListTag()
	if err := Save(Provider{ID: "codex", Contexts: map[string]int{"gpt-5.5": 600000}}); err != nil {
		t.Fatal(err)
	}
	set := CodexListTag()
	if set == base {
		t.Error("the native model's context setting is absent from the tag")
	}
	snapshot := CodexNativeContexts()
	if err := Save(Provider{ID: "codex", Contexts: map[string]int{"gpt-5.5": 400000}}); err != nil {
		t.Fatal(err)
	}
	if changed := CodexListTag(); changed == set || changed == base {
		t.Error("changing a native window did not change the tag")
	}
	if got := CodexListTagWithContexts(snapshot); got != set {
		t.Errorf("a settings edit changed the tag of the earlier snapshot: %q, want %q", got, set)
	}
	if err := Save(Provider{ID: "codex", Contexts: map[string]int{"*": 400000, "gpt-5.5": 600000}}); err != nil {
		t.Fatal(err)
	}
	wildcard := CodexListTag()
	if wildcard == set {
		t.Error("the provider-wide window is absent from the tag")
	}
	if err := SetCodexAgentsV1(true); err != nil {
		t.Fatal(err)
	}
	if v1 := CodexListTag(); v1 == wildcard || !codexcat.MarkedV1(codexcat.WithTag("", v1)) {
		t.Error("context tagging lost the multi-agent policy")
	}
	if err := SetCodexAgentsV1(false); err != nil {
		t.Fatal(err)
	}
	if err := Save(Provider{ID: "codex"}); err != nil {
		t.Fatal(err)
	}
	if reset := CodexListTag(); reset != base {
		t.Errorf("reset: tag %q, want %q", reset, base)
	}
}
