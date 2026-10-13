package enterprisebuild

import (
	"bytes"
	"debug/pe"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tc-hib/winres"
	winversion "github.com/tc-hib/winres/version"
	"github.com/yetone/magpie/internal/distribution"
)

// A PE resource round-trip is portable file validation, not a Windows GUI test.
func TestEnterpriseWindowsResourcesRepackWithoutChangingCode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\ngo 1.26.8\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rs := &winres.ResourceSet{}
	rs.SetManifest(winres.AppManifest{Description: "Magpie", DPIAwareness: winres.DPIPerMonitorV2, LongPathAware: true})
	var vi winversion.Info
	if err := vi.Set(winversion.LangDefault, winversion.LegalCopyright, "Upstream fixture copyright; MIT"); err != nil {
		t.Fatal(err)
	}
	rs.SetVersionInfo(vi)
	f, err := os.Create(filepath.Join(dir, "rsrc_windows_amd64.syso"))
	if err != nil {
		t.Fatal(err)
	}
	if err := rs.WriteObject(f, winres.ArchAMD64); err != nil {
		t.Fatal(err)
	}
	f.Close()
	base := filepath.Join(dir, "runtime.exe")
	cmd := exec.Command("go", "build", "-o", base, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PE fixture: %v %s", err, b)
	}
	_, profile, logo := fixture(t)
	prepared := filepath.Join(t.TempDir(), "icons")
	p, err := Prepare(profile, logo, prepared)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(prepared, "logo.png"))
	if err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(distribution.Package{SchemaVersion: 1, Profile: p, Version: "1.0.6", BuildNumber: 8, RuntimeVersion: "0.1.1172", Target: "windows/amd64", Logo: b})
	out := filepath.Join(dir, p.Identity.ArtifactSlug+".exe")
	if err := patchWindows(base, out, prepared, p, "1.0.6", 8, config); err != nil {
		t.Fatal(err)
	}
	a, err := pe.Open(base)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	z, err := pe.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	codeA, _ := a.Section(".text").Data()
	codeZ, _ := z.Section(".text").Data()
	if !bytes.Equal(codeA, codeZ) || a.Machine != z.Machine {
		t.Fatal("resource patch recompiled/changed code")
	}
	f, err = os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	patched, err := winres.LoadFromEXE(f)
	if err != nil {
		t.Fatal(err)
	}
	c, err := distribution.DecodePackage(patched.Get(winres.RT_RCDATA, winres.Name("ENTERPRISE_PACKAGE"), 0))
	if err != nil || c.Profile.DistributionID != p.DistributionID || c.Version != "1.0.6" {
		t.Fatalf("package configuration lost: %+v %v", c, err)
	}
	m, err := winres.AppManifestFromXML(patched.Get(winres.RT_MANIFEST, winres.ID(1), winres.LCIDDefault))
	if err != nil || m.Identity.Name != p.Identity.WindowsAppID || !m.LongPathAware || m.DPIAwareness != winres.DPIPerMonitorV2 {
		t.Fatal("platform identity or base manifest changed", m, err)
	}
	if _, err := patched.GetIcon(winres.Name("APP")); err != nil {
		t.Fatal("branded icon missing", err)
	}
	versionInfo, err := winversion.FromBytes(patched.Get(winres.RT_VERSION, winres.ID(1), winres.LCIDDefault))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(&vi)
	after, _ := json.Marshal(versionInfo)
	if !bytes.Contains(before, []byte("Upstream fixture copyright; MIT")) || !bytes.Contains(after, []byte("Upstream fixture copyright; MIT")) {
		t.Fatal("rebranding removed upstream legal attribution")
	}
}
