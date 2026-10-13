package settings

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/yetone/magpie/internal/distribution"
)

func enterpriseHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func TestEnterpriseDefaultsAndDisplayOnly(t *testing.T) {
	enterpriseHome(t)
	e := ResolveEnterprise(Load())
	if e.LibraryEnabled || e.PublicPluginCatalog || e.ShowCustomProviderEntry || e.ShowPluginProviderEntry {
		t.Fatalf("defaults are not light: %+v", e)
	}
	if !EnterpriseAllowed("subscription:copilot", e) || !EnterpriseAllowed("preset:ollama", e) || EnterpriseAllowed("preset:ollama-cloud", e) {
		t.Fatal("default provider identities")
	}
}

func TestEnterpriseSavePreservesOtherFieldsAndStaleSaves(t *testing.T) {
	enterpriseHome(t)
	s := Load()
	s.Theme = "dark"
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	stale := Load()
	next := ResolveEnterprise(stale)
	next.AppName = "Acme <Assistant>"
	next.OverrideFields = []string{"appName", "libraryEnabled"}
	next.LibraryEnabled = true
	if err := SetEnterprise(&next, EnterpriseRevision(stale)); err != nil {
		t.Fatal(err)
	}
	if got := Load(); got.Theme != "dark" || ResolveEnterprise(got).AppName != next.AppName {
		t.Fatal("local update lost preferences")
	}
	stale.Theme = "light"
	if err := Save(stale); err != nil {
		t.Fatal(err)
	}
	if got := ResolveEnterprise(Load()); got.AppName != next.AppName || !got.LibraryEnabled {
		t.Fatal("a stale ordinary writer overwrote enterprise settings")
	}
	carried := Settings{}
	carried.KeepOwn(Load())
	if carried.Enterprise == nil || carried.Enterprise.AppName != next.AppName {
		t.Fatal("sync lost local overrides")
	}
}

func TestEnterpriseConcurrentSaveAndInvalidInput(t *testing.T) {
	enterpriseHome(t)
	s := Load()
	rev := EnterpriseRevision(s)
	next := ResolveEnterprise(s)
	next.OverrideFields = []string{"appName"}
	next.AppName = "Acme"
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() { errs <- SetEnterprise(&next, rev) })
	}
	wg.Wait()
	close(errs)
	good, conflict := 0, 0
	for err := range errs {
		if err == nil {
			good++
		} else if errors.Is(err, ErrEnterpriseConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if good != 1 || conflict != 1 {
		t.Fatalf("save outcomes: %d success, %d conflict", good, conflict)
	}
	for _, bad := range []Enterprise{{SchemaVersion: 2}, {SchemaVersion: 1, AppName: "\nAcme"}, {SchemaVersion: 1, Defaults: distribution.Defaults{VisibleProviders: []string{"preset:../private"}}}, {SchemaVersion: 1, LogoAsset: "sha256:../secret"}, {SchemaVersion: 1, OverrideFields: []string{"proxy"}}} {
		if err := SetEnterprise(&bad, EnterpriseRevision(Load())); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	if ResolveEnterprise(Load()).AppName != "Acme" {
		t.Fatal("invalid save changed the config")
	}
}

func TestEnterpriseDamagedSettingsAreNotEmpty(t *testing.T) {
	enterpriseHome(t)
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	bad := []byte("{broken")
	if err := os.WriteFile(Path(), bad, 0o600); err != nil {
		t.Fatal(err)
	}
	next := EnterpriseDefaults()
	if err := SetEnterprise(&next, EnterpriseRevision(Load())); err == nil {
		t.Fatal("saved over an unreadable configuration")
	}
	b, err := os.ReadFile(filepath.Join(Dir(), "settings.json"))
	if err != nil || string(b) != string(bad) {
		t.Fatal("damaged settings were overwritten")
	}
}

func TestEnterpriseResetRestoresDefaults(t *testing.T) {
	enterpriseHome(t)
	next := EnterpriseDefaults()
	next.AppName = "Acme"
	next.OverrideFields = []string{"appName"}
	if err := SetEnterprise(&next, EnterpriseRevision(Load())); err != nil {
		t.Fatal(err)
	}
	if err := SetEnterprise(nil, EnterpriseRevision(Load())); err != nil {
		t.Fatal(err)
	}
	if Load().Enterprise != nil || ResolveEnterprise(Load()).AppName != EnterpriseDefaults().AppName {
		t.Fatal("reset did not inherit package defaults")
	}
}

func TestEnterprisePackageUpgradeKeepsOnlyExplicitOverrides(t *testing.T) {
	packaged := Enterprise{SchemaVersion: 1, AppName: "New package brand", LogoAsset: "sha256:" + strings.Repeat("a", 64), Defaults: distribution.Defaults{VisibleProviders: []string{"preset:deepseek"}, LibraryEnabled: true}}
	local := Enterprise{SchemaVersion: 1, AppName: "Employee choice", OverrideFields: []string{"appName", "publicPluginCatalog"}}
	effective := resolveEnterprise(Settings{Enterprise: &local}, packaged)
	if effective.AppName != local.AppName || effective.LogoAsset != packaged.LogoAsset || !effective.LibraryEnabled || effective.PublicPluginCatalog || !slices.Equal(effective.VisibleProviders, packaged.VisibleProviders) {
		t.Fatalf("package upgrade lost inheritance/overrides: %+v", effective)
	}
	if reset := resolveEnterprise(Settings{}, packaged); reset.AppName != packaged.AppName || reset.LogoAsset != packaged.LogoAsset {
		t.Fatal("reset discarded the enterprise package brand")
	}
	local.OverrideFields = nil
	legacy := resolveEnterprise(Settings{Enterprise: &local}, packaged)
	if legacy.AppName != local.AppName || legacy.LibraryEnabled {
		t.Fatal("legacy explicit overlay was unexpectedly converted to inheritance")
	}
}
