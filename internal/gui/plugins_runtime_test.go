package gui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/plugin"
	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/testenv"
)

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
