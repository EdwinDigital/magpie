package settings

import (
	"slices"
	"testing"

	"github.com/yetone/magpie/internal/distribution"
)

func TestEnterpriseVisibleAgentDefaultsLegacyAndEmpty(t *testing.T) {
	enterpriseHome(t)
	want := distribution.DefaultAgentIDs()
	if got := ResolveEnterprise(Load()).VisibleAgents; !slices.Equal(got, want) {
		t.Fatalf("default visible agents: %v", got)
	}
	legacy := EnterpriseDefaults()
	legacy.VisibleAgents = nil
	legacy.OverrideFields = nil
	if err := SetEnterprise(&legacy, EnterpriseRevision(Load())); err != nil {
		t.Fatal(err)
	}
	if got := ResolveEnterprise(Load()).VisibleAgents; !slices.Equal(got, want) {
		t.Fatalf("legacy omission did not inherit defaults: %v", got)
	}
	legacy.VisibleAgents = []string{}
	legacy.OverrideFields = []string{"visibleAgents"}
	if err := SetEnterprise(&legacy, EnterpriseRevision(Load())); err != nil {
		t.Fatal(err)
	}
	got := ResolveEnterprise(Load()).VisibleAgents
	if got == nil || len(got) != 0 || AgentDiscoveryAllowed("codex") {
		t.Fatalf("explicit empty must stay empty: %v", got)
	}
	legacy.VisibleAgents = []string{"codex", "codex", "workbuddy"}
	if err := SetEnterprise(&legacy, EnterpriseRevision(Load())); err != nil {
		t.Fatal(err)
	}
	if got := Load().Enterprise.VisibleAgents; len(got) != 2 {
		t.Fatalf("not deduplicated: %v", got)
	}
	if !AgentDiscoveryAllowed("codex@wsl:Ubuntu") || AgentDiscoveryAllowed("dsh") || !ProviderDiscoveryAllowed("subscription:copilot") || ProviderDiscoveryAllowed("preset:ollama-cloud") {
		t.Fatal("agent/provider scan selections are not independent and scoped")
	}
}
