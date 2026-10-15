package gui

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/plugin"
	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/testenv"
)

func TestEnterpriseZIPPluginFriendlyNameAndModels(t *testing.T) {
	bun := testenv.Bun(t)
	for _, display := range []string{"", "Company Gateway"} {
		t.Run("display="+display, func(t *testing.T) {
			sandboxHome(t)
			t.Setenv("MAGPIE_BUN", bun)
			plugin.Restart()
			t.Cleanup(plugin.Settle)
			e := settings.EnterpriseDefaults()
			e.VisibleProviders = []string{}
			e.OverrideFields = []string{"visibleProviders"}
			if err := settings.SetEnterprise(&e, settings.EnterpriseRevision(settings.Load())); err != nil {
				t.Fatal(err)
			}
			var b bytes.Buffer
			z := zip.NewWriter(&b)
			pkg, _ := json.Marshal(map[string]string{"name": "opencode-private-user-deadbeef-auth", "displayName": display, "main": "index.mjs", "version": "1.2.3"})
			source := `export default async () => ({
  auth: {provider:"private-user-deadbeef",methods:[{type:"api",label:"API key"}]},
  config: async cfg => { cfg.provider ??= {}; cfg.provider["private-user-deadbeef"]={name:"Private Models",npm:"@ai-sdk/openai-compatible",api:"https://example.invalid/v1",models:{"test-model":{id:"test-model",name:"Test Model",limit:{context:32768,output:4096}}}} }
})`
			for name, data := range map[string][]byte{"package.json": pkg, "index.mjs": []byte(source)} {
				w, err := z.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := w.Write(data); err != nil {
					t.Fatal(err)
				}
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			mux := Handler(nil, nil)
			if got := uploadPluginRequest(t, mux, "company.zip", b.Bytes(), true, 1); got.Code != http.StatusOK {
				t.Fatalf("ZIP install: %d %s", got.Code, got.Body)
			}
			if err := os.WriteFile(plugin.AuthPath(), []byte(`{"private-user-deadbeef":{"type":"api","key":"fixture-key"}}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := plugin.Providers(context.Background()); err != nil {
				t.Fatal(err)
			}
			got := enterpriseRequest(t, mux, "GET", "/api/plugins", nil)
			var state struct {
				Plugins []struct {
					Name            string   `json:"name"`
					Spec            string   `json:"spec"`
					HiddenProviders []string `json:"hiddenProviders"`
				} `json:"plugins"`
			}
			if err := json.Unmarshal(got.Body.Bytes(), &state); err != nil || len(state.Plugins) != 1 {
				t.Fatalf("installed metadata: %s %v", got.Body, err)
			}
			want := display
			if want == "" {
				want = "Private Models"
			}
			if state.Plugins[0].Name != want {
				t.Fatalf("ZIP plugin shows a technical package ID instead of its friendly name: got %q, want %q", state.Plugins[0].Name, want)
			}
			if len(state.Plugins[0].HiddenProviders) != 1 || settings.ProviderDiscoveryAllowed("plugin:private-user-deadbeef") {
				t.Fatal("ZIP installation bypassed the explicit enterprise supplier selection")
			}
			found := false
			for _, c := range enterpriseProviders() {
				if c.ID == "plugin:private-user-deadbeef" && c.Name == "Private Models" && c.Available {
					found = true
				}
			}
			if !found {
				t.Fatal("ZIP supplier is missing from enterprise configuration")
			}
			e.VisibleProviders = []string{"plugin:private-user-deadbeef"}
			if err := settings.SetEnterprise(&e, settings.EnterpriseRevision(settings.Load())); err != nil {
				t.Fatal(err)
			}
			got = enterpriseRequest(t, mux, "GET", "/api/providers", nil)
			var providers struct {
				Providers []struct {
					ID     string `json:"id"`
					Models []struct {
						ID string `json:"id"`
					} `json:"models"`
				} `json:"providers"`
			}
			if err := json.Unmarshal(got.Body.Bytes(), &providers); err != nil {
				t.Fatal(err)
			}
			found = false
			for _, p := range providers.Providers {
				if p.ID == "private-user-deadbeef" && len(p.Models) == 1 && p.Models[0].ID == "test-model" {
					found = true
				}
			}
			if !found {
				t.Fatalf("selected ZIP supplier has no configurable models: %s", got.Body)
			}
			if err := plugin.SetOff(state.Plugins[0].Spec, true); err != nil {
				t.Fatal(err)
			}
			got = enterpriseRequest(t, mux, "GET", "/api/plugins", nil)
			if err := json.Unmarshal(got.Body.Bytes(), &state); err != nil || state.Plugins[0].Name != want {
				t.Fatalf("switching off a ZIP plugin lost its friendly name: %s %v", got.Body, err)
			}
		})
	}
}

func TestPluginsStateReportsMissingRuntimeForPreviouslyInstalledProvider(t *testing.T) {
	sandboxHome(t)
	t.Setenv("MAGPIE_BUN", "")
	plugin.Restart()
	t.Cleanup(plugin.Settle)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.mjs"), []byte(`export default () => ({})`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(settings.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(plugin.List{Plugins: []plugin.Entry{{Spec: dir}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settings.Dir(), "plugins.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if plugin.HasBun() {
		t.Fatal("fixture already has a plugin runtime")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := pluginsState(ctx, nil)
	if s.Error == "" || len(s.Plugins) != 1 || s.Plugins[0].Spec != dir || s.Plugins[0].Error != s.Error {
		t.Fatalf("a missing runtime was presented as no usable subscriptions: %+v", s)
	}
}

func TestEnterpriseInstalledPluginIsSelectableAndExplainsItsHiddenSupplier(t *testing.T) {
	bun := testenv.Bun(t)
	sandboxHome(t)
	t.Setenv("MAGPIE_BUN", bun)
	plugin.Restart()
	t.Cleanup(plugin.Settle)
	e := settings.EnterpriseDefaults()
	e.VisibleProviders = []string{}
	e.OverrideFields = []string{"visibleProviders"}
	if err := settings.SetEnterprise(&e, settings.EnterpriseRevision(settings.Load())); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	src := `export default async () => ({ auth: {provider:"private-models",methods:[{type:"api",label:"API key"}]},config: async cfg => { cfg.provider ??= {}; cfg.provider["private-models"]={name:"Private Models",models:{}} } })`
	if err := os.WriteFile(filepath.Join(dir, "index.mjs"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := plugin.Add(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	s := pluginsState(context.Background(), nil)
	plugin.Refreshed()
	s = pluginsState(context.Background(), nil)
	if s.Error != "" || len(s.Plugins) != 1 || len(s.Plugins[0].Providers) != 1 || len(s.Plugins[0].HiddenProviders) != 1 {
		t.Fatalf("installed plugin did not explain its hidden supplier: %+v", s)
	}
	found := false
	for _, c := range enterpriseProviders() {
		if c.ID == "plugin:private-models" && c.Available {
			found = true
		}
	}
	if !found {
		t.Fatal("hidden supplier cannot be selected without reinstalling")
	}
	e.VisibleProviders = []string{"plugin:private-models"}
	if err := settings.SetEnterprise(&e, settings.EnterpriseRevision(settings.Load())); err != nil {
		t.Fatal(err)
	}
	if s = pluginsState(context.Background(), nil); s.Error != "" || len(s.Plugins[0].HiddenProviders) != 0 {
		t.Fatalf("selection did not immediately unhide the supplier: %+v", s)
	}
}
