package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type zipFixture struct {
	name, body string
	mode       os.FileMode
}

func archiveFixture(t *testing.T, files ...zipFixture) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, f := range files {
		h := &zip.FileHeader{Name: f.name, Method: zip.Deflate}
		mode := f.mode
		if mode == 0 {
			mode = 0o600
		}
		h.SetMode(mode)
		out, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := out.Write([]byte(f.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestZIPInstallPersistsAndRemovesOnlyOwnedFiles(t *testing.T) {
	for _, prefix := range []string{"", "repo-main/"} {
		t.Run(prefix, func(t *testing.T) {
			dir := storeSandbox(t)
			t.Setenv("MAGPIE_BUN", filepath.Join(dir, "runtime-not-executed"))
			t.Cleanup(Settle)
			b := archiveFixture(t,
				zipFixture{prefix + "package.json", `{"name":"@company/example","version":"1.2.3","main":"./index.js"}`, 0},
				zipFixture{prefix + "index.js", `export default async () => ({})`, 0},
				zipFixture{"__MACOSX/._package.json", "metadata", 0},
			)
			e, err := AddZIP(context.Background(), bytes.NewReader(b), int64(len(b)))
			if err != nil {
				t.Fatal(err)
			}
			if e.Archive == "" || len(Load().Plugins) != 1 || filepath.Base(e.Spec) != "example" || PackageName(e.Spec) != "@company/example" || Version(e.Spec) != "1.2.3" {
				t.Fatalf("not installed as a persistent local package: %+v, %+v", e, Load())
			}
			got, err := os.ReadFile(filepath.Join(e.Spec, "index.js"))
			if err != nil || string(got) != `export default async () => ({})` {
				t.Fatalf("installed files were not retained: %q %v", got, err)
			}
			if _, err := Add(context.Background(), e.Spec); err != nil || Load().Plugins[0].Archive != e.Archive {
				t.Fatalf("re-adding loses archive ownership: %v %+v", err, Load())
			}
			local := filepath.Join(t.TempDir(), "index.js")
			if err := os.WriteFile(local, []byte("export default () => ({})"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Add(context.Background(), local); err != nil {
				t.Fatal(err)
			}
			if err := Remove(context.Background(), e.Spec); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(Dir(), "imports", e.Archive)); !os.IsNotExist(err) {
				t.Fatalf("owned import remains: %v", err)
			}
			if err := Remove(context.Background(), local); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(local); err != nil {
				t.Fatalf("ordinary folder/file removed: %v", err)
			}
		})
	}
}

func TestZIPRejectsUnsafeOrIncompletePackages(t *testing.T) {
	base := []zipFixture{{"package.json", `{"name":"example","main":"index.js"}`, 0}, {"index.js", "export default () => ({})", 0}}
	cases := map[string][]zipFixture{
		"missing manifest": {{"index.js", "export default () => ({})", 0}},
		"bad manifest":     {{"package.json", "null\n", 0}, {"index.js", "x", 0}},
		"missing entry":    {{"package.json", `{"main":"dist/index.js"}`, 0}},
		"command only":     {{"package.json", `{"name":"command","bin":"cli.js"}`, 0}, {"cli.js", "x", 0}},
		"multiple roots":   {{"first/package.json", `{}`, 0}, {"second/package.json", `{}`, 0}},
		"symlink":          append(append([]zipFixture{}, base...), zipFixture{"link", "../../outside", os.ModeSymlink | 0o777}),
		"device":           append(append([]zipFixture{}, base...), zipFixture{"device", "", os.ModeDevice | 0o600}),
		"duplicate":        append(append([]zipFixture{}, base...), zipFixture{"INDEX.JS", "other", 0}),
	}
	for _, name := range []string{"../outside", "a/../../outside", "/outside", `C:\outside`, `a\..\outside`, "C:/outside", "dir/file:stream", "NUL.js", "COM1", "dir/file.", "dir/file ", "a//b", "a/./b", "bad\x00name"} {
		cases[name] = append(append([]zipFixture{}, base...), zipFixture{name, "bad", 0})
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			storeSandbox(t)
			b := archiveFixture(t, files...)
			if _, err := AddZIP(context.Background(), bytes.NewReader(b), int64(len(b))); err == nil {
				t.Fatal("unsafe or incomplete plugin ZIP installed")
			}
			if len(Load().Plugins) != 0 {
				t.Fatal("failure changed installed plugins")
			}
			entries, _ := os.ReadDir(filepath.Join(Dir(), "imports"))
			if len(entries) != 0 {
				t.Fatalf("failure left extracted files: %v", entries)
			}
		})
	}
}

func TestZIPLimitsCorruptionAndCancellation(t *testing.T) {
	storeSandbox(t)
	for _, size := range []int64{0, MaxZIPSize + 1} {
		if _, err := AddZIP(context.Background(), bytes.NewReader(nil), size); err == nil {
			t.Fatal("compressed-size limit not enforced")
		}
	}
	if _, err := AddZIP(context.Background(), bytes.NewReader([]byte("not a ZIP")), 9); err == nil {
		t.Fatal("non-ZIP accepted")
	}
	b := archiveFixture(t, zipFixture{"index.js", strings.Repeat("x", maxZIPFileSize+1), 0})
	if _, err := AddZIP(context.Background(), bytes.NewReader(b), int64(len(b))); err == nil {
		t.Fatal("expanded file size not enforced")
	}
	// Forge a small declared size: extraction must still fail, without retaining
	// files, when the real stream disagrees with the central directory.
	i := bytes.Index(b, []byte("PK\x01\x02"))
	binary.LittleEndian.PutUint32(b[i+24:i+28], 1)
	if _, err := AddZIP(context.Background(), bytes.NewReader(b), int64(len(b))); err == nil {
		t.Fatal("forged expanded size accepted")
	}
	large := strings.Repeat("x", maxZIPFileSize)
	b = archiveFixture(t,
		zipFixture{"one", large, 0}, zipFixture{"two", large, 0},
		zipFixture{"three", large, 0}, zipFixture{"four", large, 0}, zipFixture{"five", "x", 0},
	)
	if _, err := AddZIP(context.Background(), bytes.NewReader(b), int64(len(b))); err == nil {
		t.Fatal("total expanded size not enforced")
	}
	files := make([]zipFixture, maxZIPEntries+1)
	for i := range files {
		files[i] = zipFixture{"dir/", "", os.ModeDir | 0o700}
	}
	b = archiveFixture(t, files...)
	if _, err := AddZIP(context.Background(), bytes.NewReader(b), int64(len(b))); err == nil {
		t.Fatal("entry count limit not enforced")
	}
	b = archiveFixture(t, zipFixture{"index.js", "x", 0})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := AddZIP(ctx, bytes.NewReader(b), int64(len(b))); err != context.Canceled {
		t.Fatalf("cancelled upload: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(Dir(), "imports"))
	if len(entries) != 0 {
		t.Fatalf("failed import left files: %v", entries)
	}
}

func TestZIPRunsThroughExistingPluginHost(t *testing.T) {
	sandbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	b := archiveFixture(t,
		zipFixture{"package.json", `{"name":"archive-example","main":"index.js"}`, 0},
		zipFixture{"index.js", `export default async () => ({
  auth: { provider: "archive-example", methods: [{type: "api", label: "API key"}] },
  config: async cfg => { cfg.provider ??= {}; cfg.provider["archive-example"] = {name: "Archive Example", models: {}} }
})`, 0},
	)
	e, err := AddZIP(ctx, bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		loaded, err := Plugins(ctx)
		if err != nil || len(loaded) != 1 || loaded[0].Error != "" || loaded[0].Spec != e.Spec {
			t.Fatalf("ZIP did not load in existing host: %+v %v", loaded, err)
		}
		ps, err := Providers(ctx)
		if err != nil || len(ps) != 1 || ps[0].ID != "archive-example" {
			t.Fatalf("ZIP provider not exposed: %+v %v", ps, err)
		}
		Restart()
	}
}
