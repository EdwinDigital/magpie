package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/testenv"
)

func agentDiscoveryPolicy(t *testing.T, ids []string) {
	t.Helper()
	testenv.SetHome(t, t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	e := settings.EnterpriseDefaults()
	e.VisibleAgents = ids
	e.OverrideFields = []string{"visibleAgents"}
	if err := settings.SetEnterprise(&e, settings.EnterpriseRevision(settings.Load())); err != nil {
		t.Fatal(err)
	}
}

func TestEnterpriseHiddenAgentNeverRunsDetectorOrInstallProbe(t *testing.T) {
	agentDiscoveryPolicy(t, []string{})
	called := 0
	a := &Agent{ID: "codex", detect: func() bool { called++; return true }}
	if a.Detected() {
		t.Fatal("unselected agent was detected")
	}
	if out := installsOf([]*Agent{a}, runtime.GOOS, true); len(out) != 0 {
		t.Fatalf("hidden install entry: %+v", out)
	}
	if called != 0 {
		t.Fatalf("hidden detector ran %d times", called)
	}
	e := settings.EnterpriseDefaults()
	e.VisibleAgents = []string{"codex"}
	e.OverrideFields = []string{"visibleAgents"}
	if err := settings.SetEnterprise(&e, settings.EnterpriseRevision(settings.Load())); err != nil {
		t.Fatal(err)
	}
	if !a.Detected() || called != 1 {
		t.Fatal("selection did not immediately enable detection")
	}
}

func TestEnterpriseWSLProbeScriptUsesCurrentSelection(t *testing.T) {
	agentDiscoveryPolicy(t, []string{"codex"})
	s := discoveryProbeScript(settings.AgentDiscoveryAllowed)
	if !strings.Contains(s, "command -v codex") || strings.Contains(s, "command -v gemini") || strings.Contains(s, ".omp/profiles") || strings.Contains(s, "command -v dsh") {
		t.Fatalf("probe includes hidden agent kinds: %s", s)
	}
	e := settings.EnterpriseDefaults()
	e.VisibleAgents = []string{"gemini"}
	e.OverrideFields = []string{"visibleAgents"}
	if err := settings.SetEnterprise(&e, settings.EnterpriseRevision(settings.Load())); err != nil {
		t.Fatal(err)
	}
	s = discoveryProbeScript(settings.AgentDiscoveryAllowed)
	if strings.Contains(s, "command -v codex") || !strings.Contains(s, "command -v gemini") {
		t.Fatal("probe policy is frozen until restart")
	}
}

func TestEnterpriseHiddenDesktopKeepsMetadataAndExplicitAccess(t *testing.T) {
	agentDiscoveryPolicy(t, []string{})
	home, _ := os.UserHomeDir()
	a := claudeDesktop(home)
	if a.expand == nil || a.Name != "Claude Desktop" || a.Path != "" || a.Detected() {
		t.Fatalf("hidden descriptor: %+v", a)
	}
	full, err := Find("claude-app")
	if err != nil || full.expand != nil || full.ID != a.ID || full.Path == "" || !strings.HasPrefix(full.Path, filepath.Clean(home)) {
		t.Fatalf("explicit lookup no longer works: %+v %v", full, err)
	}
}
