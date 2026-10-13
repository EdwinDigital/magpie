package enterprisebuild

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/distribution"
)

func runtimeFixture(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	module := "module github.com/yetone/magpie/repackfixture\n\ngo 1.26.8\nrequire github.com/yetone/magpie v0.0.0\nreplace github.com/yetone/magpie => " + root + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(module), 0o600); err != nil {
		t.Fatal(err)
	}
	code := `package main
import("encoding/json";"os";"github.com/yetone/magpie/internal/distribution";"github.com/yetone/magpie/internal/appdir")
func main(){ json.NewEncoder(os.Stdout).Encode(map[string]any{"name":distribution.Name(),"exe":distribution.ExecutableName(),"version":distribution.PackageVersion("public"),"config":appdir.Config(),"cache":appdir.Cache(),"port":distribution.GatewayPort(),"scheme":distribution.Scheme(),"profile":distribution.Current()}) }`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-mod=mod", "-trimpath", "-ldflags", "-X github.com/yetone/magpie/internal/distribution.resourcePackage=1", "-o", filepath.Join(dir, "runtime"), ".")
	cmd.Dir = dir
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("runtime fixture: %v %s", err, b)
	}
	if err := copyFile("../../build/darwin/Info.plist", filepath.Join(dir, "Info.plist"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := Runtime{SchemaVersion: 1, Format: "package-resources-v1", SourceCommit: strings.Repeat("a", 40), Version: "0.1.1172", Target: "darwin/" + runtime.GOARCH}
	r.SHA256, _ = fileHash(filepath.Join(dir, "runtime"))
	r.PlistSHA256, _ = fileHash(filepath.Join(dir, "Info.plist"))
	b, _ := json.Marshal(r)
	if err := os.WriteFile(filepath.Join(dir, "runtime-metadata.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestEnterpriseRepackWithoutCompilerAndIndependentIdentity(t *testing.T) {
	base := runtimeFixture(t)
	before, _ := os.ReadFile(filepath.Join(base, "runtime"))
	// A real process witness makes compilation or source/Git access fail.
	tools := t.TempDir()
	for _, name := range []string{"go", "git"} {
		if err := os.WriteFile(filepath.Join(tools, name), []byte("#!/bin/sh\necho unexpected-compiler-or-source >&2\nexit 77\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	ports := map[float64]bool{}
	for _, name := range []string{"msai-client", "other-client"} {
		p, profile, logo := fixture(t)
		p.DistributionID = name
		p.Identity.ArtifactSlug = name
		p.Identity.URLScheme = name
		p.Identity.MacBundleID = "com.test." + name
		p.Identity.WindowsAppID = "com.test." + name
		p.Brand.DisplayName = name
		p.EnterpriseDefaults.VisibleAgents = []string{}
		b, _ := json.Marshal(p)
		if err := os.WriteFile(profile, b, 0o600); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(t.TempDir(), "out")
		r, err := Build(context.Background(), Options{BaseRuntime: base, ProfilePath: profile, LogoPath: logo, Output: out, Target: "darwin/" + runtime.GOARCH, Version: "1.0.6", BuildNumber: 8, Unsigned: true})
		if err != nil {
			t.Fatal(err)
		}
		exe := filepath.Join(out, name+".app", "Contents", "MacOS", name)
		got, _ := os.ReadFile(exe)
		if !bytes.Equal(before, got) {
			t.Fatal("repack recompiled or changed executable machine code")
		}
		cmd := exec.Command(exe)
		b, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("packaged runtime: %v %s", err, b)
		}
		var state map[string]any
		if err := json.Unmarshal(b, &state); err != nil {
			t.Fatal(err)
		}
		if state["exe"] != name || state["name"] != name || state["version"] != "1.0.6" || filepath.Base(state["config"].(string)) != name || filepath.Base(state["cache"].(string)) != name || state["scheme"] != name {
			t.Fatalf("public identity leaked: %s", b)
		}
		port := state["port"].(float64)
		if port == 3425 || ports[port] {
			t.Fatal("gateway namespaces collide")
		}
		ports[port] = true
		if r.RuntimeVersion != "0.1.1172" || r.RuntimeSHA256 == "" {
			t.Fatal("runtime provenance lost")
		}
		if _, err := Build(context.Background(), Options{BaseRuntime: base, ProfilePath: profile, LogoPath: logo, Output: out, Target: r.Target, Version: "1.0.6", BuildNumber: 8, Unsigned: true}); err == nil {
			t.Fatal("overwrote installation")
		}
		resource := filepath.Join(out, name+".app", "Contents", "Resources", distribution.PackageResource)
		if err := os.Remove(resource); err != nil {
			t.Fatal(err)
		}
		if b, err := exec.Command(exe).CombinedOutput(); err == nil || !bytes.Contains(b, []byte("enterprise package")) {
			t.Fatalf("missing config silently became public: %v %s", err, b)
		}
	}
	if err := os.WriteFile(filepath.Join(base, "runtime"), append(bytes.Clone(before), 0), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRuntime(base); err == nil {
		t.Fatal("tampered base accepted")
	}
}
