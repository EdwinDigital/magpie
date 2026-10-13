//go:build !windows

package provider

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/settings"
)

func TestEnterpriseCopilotDiscoveryIsIndependentOfVisibleAgents(t *testing.T) {
	limitsHome(t)
	e := settings.EnterpriseDefaults()
	e.VisibleAgents = []string{"workbuddy"}
	e.OverrideFields = []string{"visibleAgents"}
	if err := settings.SetEnterprise(&e, settings.EnterpriseRevision(settings.Load())); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COPILOT_HOME", t.TempDir())
	writeFile(t, filepath.Join(copilotCLIHome(), "config.json"), map[string]any{
		"lastLoggedInUser": map[string]string{"host": "https://github.com", "login": "synthetic-user"},
		"copilotTokens":    map[string]string{"https://github.com:synthetic-user": "synthetic-token"},
	})
	if settings.AgentDiscoveryAllowed("copilot") || settings.AgentDiscoveryAllowed("vscode") {
		t.Fatal("reading a visible supplier enabled hidden agent scanning")
	}
	if a, ok := copilotCLILogin(); !ok || a.User != "synthetic-user" || a.Token != "synthetic-token" {
		t.Fatalf("visible Copilot did not read an existing hidden CLI's sign-in: %+v %v", a, ok)
	}
	cfg := t.TempDir()
	writeFile(t, filepath.Join(cfg, "github-copilot", "apps.json"), map[string]any{"github.com:synthetic-app": map[string]string{"user": "synthetic-editor", "oauth_token": "synthetic-editor-token"}})
	if a, _ := copilotEditorLogins(cfg); a == nil || a.User != "synthetic-editor" {
		t.Fatal("visible Copilot did not read an existing hidden editor's sign-in")
	}
	t.Setenv("MAGPIE_NO_ACCOUNTS", "1")
	if _, ok := copilotCLILogin(); ok {
		t.Fatal("enterprise Copilot bypassed the global privacy switch")
	}
	if a, b := copilotEditorLogins(cfg); a != nil || b != nil {
		t.Fatal("enterprise editor discovery bypassed the global privacy switch")
	}
}

func TestEnterpriseUnselectedNonCopilotSourcesReadNoCredentialAndRunNoCLI(t *testing.T) {
	limitsHome(t)
	e := settings.EnterpriseDefaults()
	e.VisibleAgents = []string{}
	e.VisibleProviders = []string{}
	e.OverrideFields = []string{"visibleAgents", "visibleProviders"}
	if err := settings.SetEnterprise(&e, settings.EnterpriseRevision(settings.Load())); err != nil {
		t.Fatal(err)
	}
	ForgetAccounts()
	t.Cleanup(ForgetAccounts)
	home, _ := os.UserHomeDir()
	ran := recordingTools(t, []string{"security", "secret-tool", "claude", "codex", "cursor-agent", "devin", "grok", "kiro-cli", "copilot"})
	paths := []string{claudeCredentialsPath(), filepath.Join(home, ".claude.json"), codexAuthPath(), cursorAuthFile(), DevinCredentialsPath(), filepath.Join(geminiDir(), "oauth_creds.json"), zcodeCredentialsPath(), cmdAuthPath(), filepath.Join(kiroIDEDir(), "kiro-auth-token.json"), wbAuthPath(wbCN), wbAuthPath(wbAI)}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	opened := openWatch(t, paths)
	if _, err := os.ReadFile(codexAuthPath()); err != nil {
		t.Fatal(err)
	}
	if got := opened(); len(got) == 0 {
		t.Fatal("file-open witness was not active")
	}
	before := len(opened())
	Accounts()
	All()
	Logins("")
	if got := opened(); len(got) != before {
		t.Fatalf("unselected credentials were read: %v", got[before:])
	}
	if got := ran(); len(got) != 0 {
		t.Fatalf("unselected source CLI ran: %v", got)
	}
	if !settings.ProviderDiscoveryAllowed("subscription:copilot") || settings.AgentDiscoveryAllowed("copilot") {
		t.Fatal("default supplier and agent selection were conflated")
	}
}

func TestEnterpriseHiddenSourcesStillServeExplicitCredentialReads(t *testing.T) {
	kiroSandbox(t)
	e := settings.EnterpriseDefaults()
	e.VisibleAgents = []string{}
	e.VisibleProviders = []string{}
	e.OverrideFields = []string{"visibleAgents", "visibleProviders"}
	if err := settings.SetEnterprise(&e, settings.EnterpriseRevision(settings.Load())); err != nil {
		t.Fatal(err)
	}
	writeFile(t, wbAuthPath(wbCN), map[string]any{"account": map[string]any{"uid": "synthetic-user"}, "auth": map[string]any{"accessToken": "synthetic-access", "refreshToken": "synthetic-refresh", "expiresAt": time.Now().Add(time.Hour).UnixMilli()}})
	if _, _, ok := wbOwnDiscovered(wbCN); ok {
		t.Fatal("hidden WorkBuddy auto-discovered")
	}
	if _, c, ok := wbOwn(wbCN); !ok || c.Access != "synthetic-access" {
		t.Fatal("visibility denied explicit WorkBuddy credentials")
	}
	writeFile(t, cmdAuthPath(), map[string]any{"apiKey": "synthetic-key", "userId": "synthetic-user", "userName": "test"})
	if _, _, ok := cmdOwnDiscovered(); ok {
		t.Fatal("hidden Command Code auto-discovered")
	}
	if _, c, ok := cmdOwn(); !ok || c.APIKey != "synthetic-key" {
		t.Fatal("visibility denied explicit Command Code credentials")
	}
	t.Setenv("ZCODE_CREDENTIAL_SECRET", "test-secret")
	writeFile(t, zcodeCredentialsPath(), map[string]any{"oauth:zai:user_info": zcodeEncrypt(t, `{"user_id":"u1","email":"synthetic@example.invalid"}`), "account-provider:coding-plan:account:zai-individual-coding-plan:account:abc:api-key": zcodeEncrypt(t, "synthetic.zcode")})
	if _, _, ok := zcodeOwnDiscovered(); ok {
		t.Fatal("hidden ZCode auto-discovered")
	}
	if _, c, ok := zcodeOwn(); !ok || c.Key != "synthetic.zcode" {
		t.Fatal("visibility denied explicit ZCode credentials")
	}
	writeFile(t, filepath.Join(GrokHome(), "auth.json"), map[string]any{"https://auth.x.ai::u1": map[string]any{"key": "synthetic-grok", "email": "test@example.invalid", "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}})
	if _, ok := discoveredGrokCredential(); ok {
		t.Fatal("hidden Grok auto-discovered")
	}
	if c, err := grokAccessToken(GrokHome(), "", false); err != nil || c.Key != "synthetic-grok" {
		t.Fatalf("visibility denied explicit Grok request: %+v %v", c, err)
	}
	writeKiroCLI(t, map[string]any{"kirocli:social:token": map[string]any{"access_token": "synthetic-kiro", "refresh_token": "rt", "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), "provider": "google", "profile_arn": "arn:aws:codewhisperer:us-east-1:1:profile/P"}})
	if kiroOwnUser() != "" {
		t.Fatal("hidden Kiro auto-discovered")
	}
	if a, err := KiroAuthOf(context.Background(), "", "", false); err != nil || a.Token != "synthetic-kiro" {
		t.Fatalf("visibility denied explicit Kiro request: %+v %v", a, err)
	}
}

func TestEnterpriseAutomaticProviderPredicateKeepsStoredConfiguration(t *testing.T) {
	limitsHome(t)
	e := settings.EnterpriseDefaults()
	e.OverrideFields = []string{"visibleProviders"}
	e.VisibleProviders = []string{"preset:deepseek"}
	if err := settings.SetEnterprise(&e, settings.EnterpriseRevision(settings.Load())); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		p    Provider
		want bool
	}{
		{Provider{ID: "custom-one"}, false}, {Provider{ID: "custom-two", Preset: "deepseek"}, true},
		{Provider{ID: "ollama", Preset: "ollama"}, true}, {Provider{ID: "ollama-cloud", Preset: "ollama-cloud"}, false},
		{Provider{ID: "copilot", Account: &Account{Agent: "copilot"}}, true}, {Provider{ID: "codex", Account: &Account{Agent: "codex"}}, false},
	} {
		if got := automaticProviderAllowed(tc.p); got != tc.want {
			t.Errorf("%s: %v want %v", tc.p.ID, got, tc.want)
		}
	}
	p := Provider{ID: "retained", Name: "Retained", Chat: "http://retained.invalid/v1", Keys: []KeyAccount{{Key: "synthetic-key"}}, Models: []string{"test-model"}}
	if err := store(file{Providers: []Provider{p}}); err != nil {
		t.Fatal(err)
	}
	Changed()
	got, ok := find(All(), p.ID)
	if !ok || len(got.Keys) != 1 || got.Keys[0].Key != "synthetic-key" || automaticProviderAllowed(got) {
		t.Fatalf("hiding an automatic source erased stored provider data: %+v", got)
	}
}
