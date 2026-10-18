package sessions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/settings"
)

func TestEnterpriseSessionDiscoverySkipsHiddenStoresBeforeReading(t *testing.T) {
	setup(t)
	t.Setenv("CODEBUDDY_CONFIG_DIR", filepath.Join(t.TempDir(), "codebuddy"))
	buddy := filepath.Join(CodeBuddyDir(), "projects", "fixture", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(buddy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(buddy, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if len(codebuddyFiles()) == 0 {
		t.Fatal("fixture contains no CodeBuddy native store")
	}
	if len(ccFiles("claude", ClaudeDir())) == 0 || len(codexFiles()) == 0 {
		t.Fatal("session fixture contains no native stores")
	}
	e := settings.EnterpriseDefaults()
	e.OverrideFields = []string{"visibleAgents"}
	e.VisibleAgents = []string{"codex"}
	if err := settings.SetEnterprise(&e, settings.EnterpriseRevision(settings.Load())); err != nil {
		t.Fatal(err)
	}
	fs := allFiles()
	if len(fs) == 0 {
		t.Fatal("selected Codex sessions disappeared")
	}
	for _, f := range fs {
		if f.agent != "codex" {
			t.Fatal("hidden session source enumerated", f.agent)
		}
	}
	for _, s := range List(All) {
		if s.Agent != "codex" {
			t.Fatal("hidden sessions listed", s.Agent)
		}
	}
	e.VisibleAgents = []string{}
	if err := settings.SetEnterprise(&e, settings.EnterpriseRevision(settings.Load())); err != nil {
		t.Fatal(err)
	}
	old := WSLHomes
	calls := 0
	WSLHomes = func() []WSLHome { calls++; return nil }
	t.Cleanup(func() { WSLHomes = old })
	if fs := allFiles(); len(fs) != 0 {
		t.Fatal("all hidden sources still enumerate", len(fs))
	}
	if fs := callSources(); len(fs) != 0 {
		t.Fatal("usage calls still enumerate hidden sources", len(fs))
	}
	if dirs := Dirs(); len(dirs) != 0 {
		t.Fatal("hidden source directories queried", dirs)
	}
	if calls != 0 {
		t.Fatal("unselected WSL roots probed", calls)
	}
	read := false
	selectedFiles("opencode", func() []file { read = true; return nil })
	if read {
		t.Fatal("hidden reader was invoked before filtering")
	}
	if got := List(All); len(got) != 0 {
		t.Fatal("old indexed sessions remained after hiding", got)
	}
	// This is discovery policy, not a deletion or direct-file access policy.
	if len(codexFiles()) == 0 {
		t.Fatal("hiding deleted native sessions")
	}
}
