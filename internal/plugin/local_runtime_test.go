package plugin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/testenv"
)

func localRuntimeFixture(t *testing.T, kind string) func(context.Context) (Entry, error) {
	t.Helper()
	source := `export default async () => ({
  auth: { provider: "local-example", methods: [{type: "api", label: "API key"}] },
  config: async cfg => { cfg.provider ??= {}; cfg.provider["local-example"] = {name: "Local Example", models: {}} }
})`
	if kind == "zip" {
		b := archiveFixture(t,
			zipFixture{"package.json", `{"name":"local-example","main":"index.mjs"}`, 0},
			zipFixture{"index.mjs", source, 0},
		)
		return func(ctx context.Context) (Entry, error) { return AddZIP(ctx, bytes.NewReader(b), int64(len(b))) }
	}
	dir := t.TempDir()
	entry := filepath.Join(dir, "index.mjs")
	if err := os.WriteFile(entry, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if kind == "folder" {
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"local-example","main":"index.mjs"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		entry = dir
	}
	return func(ctx context.Context) (Entry, error) { return Add(ctx, entry) }
}

func TestLocalProviderInstallPreparesRuntimeInFreshCache(t *testing.T) {
	for _, kind := range []string{"file", "folder", "zip"} {
		t.Run(kind, func(t *testing.T) {
			bunHome(t, BunVersion, time.Now())
			t.Cleanup(Settle)
			asked := bunReleases(t, []byte("fixture runtime"))
			install := localRuntimeFixture(t, kind)
			if HasBun() {
				t.Fatal("fixture did not start with a fresh runtime cache")
			}
			if _, err := install(context.Background()); err != nil {
				t.Fatal(err)
			}
			if !HasBun() || asked.Load() != 1 {
				t.Fatalf("local %s reported installed without preparing its runtime: available=%v downloads=%d", kind, HasBun(), asked.Load())
			}
			if _, err := install(context.Background()); err != nil || asked.Load() != 1 {
				t.Fatalf("reinstall did not reuse its runtime: %v downloads=%d", err, asked.Load())
			}
		})
	}
}

func TestLocalProviderInstallRuntimeFailureKeepsInstalledList(t *testing.T) {
	for _, kind := range []string{"file", "folder", "zip"} {
		t.Run(kind, func(t *testing.T) {
			bunHome(t, BunVersion, time.Now())
			t.Cleanup(Settle)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "runtime unavailable", http.StatusServiceUnavailable)
			}))
			t.Cleanup(srv.Close)
			old := bunRelease
			bunRelease = srv.URL
			t.Cleanup(func() { bunRelease = old })
			if err := os.MkdirAll(settings.Dir(), 0o700); err != nil {
				t.Fatal(err)
			}
			writeList(t, `{"plugins":[{"spec":"retained","off":true}]}`)
			if _, err := localRuntimeFixture(t, kind)(context.Background()); err == nil {
				t.Fatal("local provider installation succeeded without an available runtime")
			}
			if l := Load(); len(l.Plugins) != 1 || l.Plugins[0].Spec != "retained" || !l.Plugins[0].Off {
				t.Fatalf("failed runtime preparation changed installed entries: %+v", l)
			}
			imports, err := os.ReadDir(filepath.Join(Dir(), "imports"))
			if err != nil && !os.IsNotExist(err) || len(imports) != 0 {
				t.Fatalf("failed installation left an owned ZIP behind: %v %v", imports, err)
			}
		})
	}
}

func TestLocalInProcessPluginInstallNeedsNoRuntime(t *testing.T) {
	for _, kind := range []string{"middleware", "agent"} {
		t.Run(kind, func(t *testing.T) {
			bunHome(t, BunVersion, time.Now())
			t.Cleanup(Settle)
			asked := bunReleases(t, []byte("must not be downloaded"))
			dir := t.TempDir()
			manifest := `{"name":"in-process","magpie":{"` + kind + `":"index.js"}}`
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "index.js"), []byte(`export default () => ({})`), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Add(context.Background(), dir); err != nil {
				t.Fatal(err)
			}
			if HasBun() || asked.Load() != 0 {
				t.Fatal("in-process plugin unnecessarily installed a host runtime")
			}
		})
	}
}

func TestFreshLocalProviderInstallLoadsDownloadedRuntime(t *testing.T) {
	real := testenv.Bun(t)
	exe, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	bunHome(t, BunVersion, time.Now())
	t.Cleanup(Settle)
	asked := bunReleases(t, exe)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := localRuntimeFixture(t, "zip")(ctx); err != nil {
		t.Fatal(err)
	}
	if !HasBun() || asked.Load() != 1 {
		t.Fatal("fresh ZIP installation did not prepare its runtime")
	}
	for i := 0; i < 2; i++ {
		ps, err := Providers(ctx)
		if err != nil || len(ps) != 1 || ps[0].ID != "local-example" || len(ps[0].Methods) != 1 {
			t.Fatalf("downloaded runtime did not expose the ZIP's provider: %+v %v", ps, err)
		}
		Restart()
	}
}
