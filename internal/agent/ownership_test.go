package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/desktopdir"
	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/testenv"
)

func enterpriseWiringSandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	testenv.SetHome(t, home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("WORKBUDDY_CONFIG_DIR", "")
	t.Setenv("MAGPIE_ADDR", "127.0.0.1:39551")
	id, key, prof := magpieID, gateway.Token, desktopProfileID
	magpieID = "magpie-enterprise-msai-client"
	gateway.Token = magpieID + "~"
	desktopProfileID = wiringProfileID(magpieID)
	t.Cleanup(func() { magpieID, gateway.Token, desktopProfileID = id, key, prof })
	return home
}

func TestEnterpriseHiddenCodexShutdownLeavesItsFailoverWiring(t *testing.T) {
	home := enterpriseWiringSandbox(t)
	a := codex(home)
	body := "model = \"gpt-5.5\"\nopenai_base_url = \"" + gateway.URL() + gateway.CodexPath + "\"\n"
	if err := os.MkdirAll(filepath.Dir(a.Path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.Path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	stash(map[string]string{"codex.failover": "1"})
	if !a.FailingOver() {
		t.Fatal("fixture does not have shutdown failover wiring")
	}
	e := settings.EnterpriseDefaults()
	e.VisibleAgents = []string{}
	e.OverrideFields = []string{"visibleAgents"}
	if err := settings.SetEnterprise(&e, settings.EnterpriseRevision(settings.Load())); err != nil {
		t.Fatal(err)
	}
	if err := EndCodexFailover(); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(a.Path); err != nil || string(b) != body {
		t.Fatalf("hidden Codex was changed while shutting down: %v %s", err, b)
	}
}

func TestEnterpriseWiringNeverClaimsOrChangesPublicAgent(t *testing.T) {
	for _, kind := range []string{"workbuddy", "codex", "codex-joined", "claude"} {
		t.Run(kind, func(t *testing.T) {
			home := enterpriseWiringSandbox(t)
			var a *Agent
			var body string
			switch kind {
			case "workbuddy":
				a = workbuddy(home)
				body = `[{"id":"fake/pro","vendor":"magpie","apiKey":"magpie-workbuddy","url":"http://127.0.0.1:3425/v1/chat/completions"}]`
			case "codex", "codex-joined":
				a = codex(home)
				body = "model_provider = \"magpie\"\nmodel = \"fake/pro\"\n[model_providers.magpie]\nbase_url = \"http://127.0.0.1:3425/v1\"\nexperimental_bearer_token = \"magpie\"\n"
				if kind == "codex-joined" {
					body = "openai_base_url = \"http://127.0.0.1:3425/backend-api/codex\"\n" + strings.TrimPrefix(body, "model_provider = \"magpie\"\n")
				}
			case "claude":
				a = claude(home)
				body = `{"model":"fake/pro","env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:3425","ANTHROPIC_AUTH_TOKEN":"magpie"}}`
			}
			if err := os.MkdirAll(filepath.Dir(a.Path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(a.Path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			isolateWiring(a)
			if a.Wired() || a.ForeignOwner() != "magpie" {
				t.Fatalf("public %s was claimed by the enterprise: wired=%v owner=%q", kind, a.Wired(), a.ForeignOwner())
			}
			for _, change := range []func() error{func() error { _, err := a.ConnectHow(); return err }, a.Disconnect, a.Reapply} {
				if err := change(); err == nil {
					t.Fatal("operation on another client's wiring was accepted")
				}
			}
			if a.Sync != nil {
				if err := a.Sync(); err != nil {
					t.Fatal(err)
				}
			}
			if b, err := os.ReadFile(a.Path); err != nil || string(b) != body {
				t.Fatalf("public agent configuration was changed: %v %s", err, b)
			}
		})
	}
}

func TestEnterpriseWorkBuddyChangesOnlyItsNamespacedModels(t *testing.T) {
	home := enterpriseWiringSandbox(t)
	if err := provider.Save(provider.Provider{ID: "fake", Name: "Fake", Chat: "http://fake.invalid/v1", Key: "synthetic", Models: []string{"pro"}}); err != nil {
		t.Fatal(err)
	}
	a := workbuddy(home)
	isolateWiring(a)
	if err := a.Apply("provider", magpieID); err != nil {
		t.Fatal(err)
	}
	if !a.Wired() {
		t.Fatal("enterprise models were not recognized as its own connection")
	}
	d, err := workbuddyRead(a.Path)
	if err != nil || len(d.models) != 1 {
		t.Fatalf("enterprise model list: %+v %v", d, err)
	}
	var own workbuddyEntry
	if json.Unmarshal(d.models[0], &own) != nil || own.Vendor != magpieID || own.APIKey != gateway.TokenFor("workbuddy") {
		t.Fatalf("enterprise used public markers: %+v", own)
	}
	public := json.RawMessage(`{"id":"public/model","vendor":"magpie","apiKey":"magpie-workbuddy","url":"http://127.0.0.1:3425/v1/chat/completions"}`)
	b, _ := json.Marshal(append(d.models, public))
	if err := os.WriteFile(a.Path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := a.Disconnect(); err != nil {
		t.Fatal(err)
	}
	d, err = workbuddyRead(a.Path)
	if err != nil || len(d.models) != 1 {
		t.Fatalf("disconnect removed another client's models: %+v %v", d, err)
	}
	var left workbuddyEntry
	_ = json.Unmarshal(d.models[0], &left)
	if left.ID != "public/model" || left.Vendor != "magpie" || left.URL != "http://127.0.0.1:3425/v1/chat/completions" || left.APIKey != "magpie-workbuddy" {
		t.Fatalf("another client's model was overwritten: %+v", left)
	}
}

func TestEnterpriseDesktopDoesNotClaimPublicProfile(t *testing.T) {
	home := enterpriseWiringSandbox(t)
	p := desktopPathsOf(desktopDirs("darwin", home, os.Getenv))
	if err := edit.SetJSON(p.config, edit.KV{Path: "deploymentMode", Value: "3p"}); err != nil {
		t.Fatal(err)
	}
	if err := edit.SetJSON(p.meta, edit.KV{Path: "appliedId", Value: publicDesktopProfileID}); err != nil {
		t.Fatal(err)
	}
	// Supply exactly these native paths; do not enumerate installed apps.
	old := desktopdir.OS
	desktopdir.OS = "darwin"
	t.Cleanup(func() { desktopdir.OS = old })
	a := claudeDesktop(home, true)
	if a.ForeignOwner() != "magpie" || a.Wired() {
		t.Fatal("public Desktop profile was claimed by the enterprise")
	}
	if _, err := a.ConnectHow(); err == nil {
		t.Fatal("enterprise replaced the public Desktop profile")
	}
	if id, _ := edit.GetJSON(p.meta, "appliedId"); id != publicDesktopProfileID {
		t.Fatal("public Desktop's applied profile changed")
	}
}

func TestWiringTokenOwnershipHasAnUnambiguousNamespace(t *testing.T) {
	enterpriseWiringSandbox(t)
	for _, key := range []string{"magpie", "magpie-workbuddy", "magpie-enterprise-msai-client-other~-workbuddy"} {
		if owner := keyOwner(key); owner == "" || owner == magpieID {
			t.Fatalf("foreign token %q belongs to %q", key, owner)
		}
	}
	if keyOwner(gateway.TokenFor("workbuddy")) != magpieID {
		t.Fatal("own token was not recognized")
	}
	if wiringProfileID(magpieID) == publicDesktopProfileID || wiringProfileID(magpieID) == wiringProfileID(magpieID+"-other") {
		t.Fatal("enterprise Desktop profiles collide")
	}
	if endpointOwner("http://localhost:39551/v1", "http://127.0.0.1:39551") != magpieID || endpointOwner("http://[::1]:39551/v1", "http://127.0.0.1:39551") != magpieID {
		t.Fatal("loopback aliases were misclassified as another client")
	}
}

func TestEnterpriseLegacyWorkBuddyConversionRequiresPrivateRecord(t *testing.T) {
	home := enterpriseWiringSandbox(t)
	if err := provider.Save(provider.Provider{ID: "fake", Name: "Fake", Chat: "http://fake.invalid/v1", Key: "synthetic", Models: []string{"pro"}}); err != nil {
		t.Fatal(err)
	}
	a := workbuddy(home)
	if err := os.MkdirAll(filepath.Dir(a.Path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := `[{"id":"fake/pro","vendor":"magpie","apiKey":"magpie-workbuddy","url":"http://127.0.0.1:39551/v1/chat/completions"}]`
	if err := os.WriteFile(a.Path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if a.ForeignOwner() != "magpie" {
		t.Fatal("same port without a private record claimed a legacy connection")
	}
	record("workbuddy", "provider", "magpie")
	isolateWiring(a)
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(a.Path); string(b) != body {
		t.Fatal("legacy wiring was migrated automatically")
	}
	if err := a.Apply("provider", magpieID); err != nil {
		t.Fatal(err)
	}
	d, err := workbuddyRead(a.Path)
	if err != nil || len(d.models) != 1 {
		t.Fatalf("legacy conversion: %+v %v", d, err)
	}
	var own workbuddyEntry
	_ = json.Unmarshal(d.models[0], &own)
	if own.Vendor != magpieID || own.APIKey != gateway.TokenFor("workbuddy") {
		t.Fatalf("legacy markers survived explicit conversion: %+v", own)
	}
}

func TestEnterpriseCodexKeepsPublicHistoryProviderAndCatalog(t *testing.T) {
	home := enterpriseWiringSandbox(t)
	if err := provider.Save(provider.Provider{ID: "fake", Name: "Fake", Chat: "http://fake.invalid/v1", Key: "synthetic", Models: []string{"pro"}}); err != nil {
		t.Fatal(err)
	}
	a := codex(home)
	if err := os.MkdirAll(filepath.Dir(a.Path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := "[model_providers.magpie]\nname = \"magpie\"\nbase_url = \"http://127.0.0.1:3425/v1\"\nexperimental_bearer_token = \"magpie\"\n"
	if err := os.WriteFile(a.Path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	publicCatalog := filepath.Join(filepath.Dir(a.Path), "magpie-models.json")
	if err := os.WriteFile(publicCatalog, []byte("public catalog"), 0o600); err != nil {
		t.Fatal(err)
	}
	isolateWiring(a)
	if err := a.Apply("model", "fake/pro"); err != nil {
		t.Fatal(err)
	}
	table, err := edit.GetTOMLTable(a.Path, "model_providers.magpie")
	if err != nil || table["base_url"] != "http://127.0.0.1:3425/v1" || table["experimental_bearer_token"] != "magpie" {
		t.Fatal("public history's table was taken over", table, err)
	}
	if p, _ := edit.GetTOMLTop(a.Path, "model_provider"); p != magpieID {
		t.Fatal("Codex was not connected through the enterprise provider")
	}
	if b, _ := os.ReadFile(publicCatalog); string(b) != "public catalog" {
		t.Fatal("public catalog was overwritten")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(a.Path), magpieID+"-models.json")); err != nil {
		t.Fatal("enterprise catalog is not namespaced", err)
	}
}

func TestEnterpriseLegacyDesktopConversionRetiresOnlyItsOldProfile(t *testing.T) {
	home := enterpriseWiringSandbox(t)
	oldOS := desktopdir.OS
	desktopdir.OS = "darwin"
	t.Cleanup(func() { desktopdir.OS = oldOS })
	p := desktopPathsOf(desktopDirs("darwin", home, os.Getenv))
	legacy := p
	legacy.profile, legacy.prof = publicDesktopProfileID, filepath.Join(p.library, publicDesktopProfileID+".json")
	if err := desktopOn(legacy); err != nil {
		t.Fatal(err)
	}
	if err := edit.SetJSON(legacy.prof, edit.KV{Path: "inferenceGatewayApiKey", Value: "magpie-claude-desktop"}); err != nil {
		t.Fatal(err)
	}
	if err := edit.SetJSON(p.meta, edit.KV{Path: "entries", Value: []map[string]string{{"id": publicDesktopProfileID, "name": "magpie"}}}); err != nil {
		t.Fatal(err)
	}
	record("claude-desktop", "provider", "magpie")
	a := claudeDesktop(home, true)
	isolateWiring(a)
	if a.ForeignOwner() != "" {
		t.Fatal("this client's recorded legacy profile cannot be explicitly converted")
	}
	if err := a.Apply("provider", magpieID); err != nil {
		t.Fatal(err)
	}
	if id, _ := edit.GetJSON(p.meta, "appliedId"); id != desktopProfileID {
		t.Fatal("new Desktop profile was not applied", id)
	}
	entries, _, err := desktopEntries(p.meta)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if desktopEntryID(e) == publicDesktopProfileID {
			t.Fatal("legacy profile can still be claimed by a public client")
		}
	}
	if _, err := os.Stat(legacy.prof); !os.IsNotExist(err) {
		t.Fatal("old owned profile was not retired", err)
	}
	if !a.Wired() {
		t.Fatal("the namespaced Desktop profile is not connected")
	}
}
