package enterprisebuild

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/distribution"
)

func fixture(t *testing.T) (distribution.Profile, string, string) {
	t.Helper()
	root := t.TempDir()
	p := distribution.Profile{SchemaVersion: 1, TenantID: "acme", DistributionID: "acme-assistant", ProfileRevision: 1}
	p.Brand.DisplayName = "Acme <Assistant>"
	p.Identity.MacBundleID = "com.acme.assistant"
	p.Identity.WindowsAppID = "com.acme.assistant"
	p.Identity.ArtifactSlug = "acme-assistant"
	p.Identity.URLScheme = "acme-assistant"
	p.Updates.Mode = "manual"
	b, _ := json.Marshal(p)
	profile := filepath.Join(root, "input.json")
	if err := os.WriteFile(profile, b, 0o600); err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 200, A: 255})
		}
	}
	var logo bytes.Buffer
	_ = png.Encode(&logo, img)
	logoPath := filepath.Join(root, "input.png")
	if err := os.WriteFile(logoPath, logo.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p, profile, logoPath
}

func TestEnterpriseWindowsManifestLookupKeysAreDistinctFromEXEName(t *testing.T) {
	p, _, _ := fixture(t)
	p.Brand.LogoAsset = distribution.AssetID([]byte("logo"))
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p.Updates.ManifestKeys = map[string]string{"test": base64.StdEncoding.EncodeToString(pub)}
	dir := t.TempDir()
	name := p.Identity.ArtifactSlug + ".exe"
	data := []byte("synthetic signed metadata fixture, not an OS-signing assertion")
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	seen := map[string]bool{}
	for _, target := range []string{"windows/amd64", "windows/arm64"} {
		r := Result{Profile: p, Signed: true, Version: "1.0.6", Target: target, Assets: map[string]distribution.Asset{name: {Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}}}
		m, err := SignManifest(r, dir, "https://download.invalid/"+strings.ReplaceAll(target, "/", "-"), "test", 1, key)
		if err != nil {
			t.Fatal(err)
		}
		lookup := p.Identity.ArtifactSlug + "-" + strings.ReplaceAll(target, "/", "-") + ".exe"
		asset, ok := m.Assets[lookup]
		if !ok || seen[lookup] || !strings.HasSuffix(asset.URL, "/"+name) {
			t.Fatal("architecture collision or process renamed", m.Assets)
		}
		seen[lookup] = true
	}
}

func TestEnterprisePrepareIsolatedAssetsAndPlist(t *testing.T) {
	p, profile, logo := fixture(t)
	out := filepath.Join(t.TempDir(), "prepared")
	p, err := Prepare(profile, logo, out)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "logo.png"))
	if err != nil || distribution.AssetID(b) != p.Brand.LogoAsset {
		t.Fatal("profile and generated logo differ")
	}
	for _, size := range []string{"16", "32", "64", "128", "256", "512", "1024"} {
		f, err := os.Open(filepath.Join(out, "icon-"+size+".png"))
		if err != nil {
			t.Fatal(err)
		}
		_, format, err := image.DecodeConfig(f)
		_ = f.Close()
		if err != nil || format != "png" {
			t.Fatal("invalid generated icon")
		}
	}
	icns, err := os.ReadFile(filepath.Join(out, "enterprise.icns"))
	if err != nil || string(icns[:4]) != "icns" || binary.BigEndian.Uint32(icns[4:8]) != uint32(len(icns)) {
		t.Fatal("invalid icon container")
	}
	if _, err := Prepare(profile, logo, out); err == nil {
		t.Fatal("overwrote prepared immutable assets")
	}
	template, err := os.ReadFile("../../build/darwin/Info.plist")
	if err != nil {
		t.Fatal(err)
	}
	plist, err := enterprisePlist(template, p, "1.2.3", 7)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(plist, []byte("Acme &lt;Assistant&gt;")) || bytes.Contains(plist, []byte("CFBundleIconName")) || !bytes.Contains(plist, []byte("com.acme.assistant")) {
		t.Fatalf("incorrect enterprise plist: %s", plist)
	}
	if _, err := Build(context.Background(), Options{Version: "1.2.3", BuildNumber: 1, Target: "windows/amd64", ProfilePath: profile, LogoPath: logo}); err == nil {
		t.Fatal("production build ran without signing configuration")
	}
}

func TestEnterprisePrepareUsesSameThemeStandardAsUploads(t *testing.T) {
	_, profile, _ := fixture(t)
	out := filepath.Join(t.TempDir(), "theme-assets")
	p, err := Prepare(profile, "testdata/microsoft-white-transparent.png", out)
	if err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"light", "dark"} {
		b, err := os.ReadFile(filepath.Join(out, "logo-"+theme+".png"))
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		c := color.NRGBAModel.Convert(img.At(300, 300)).(color.NRGBA)
		if theme == "light" && c.R > 30 || theme == "dark" && c.R < 240 {
			t.Fatalf("package theme contrast: %s %v", theme, c)
		}
		app, err := os.ReadFile(filepath.Join(out, "app-"+theme+".png"))
		if err != nil {
			t.Fatal(err)
		}
		appImg, err := png.Decode(bytes.NewReader(app))
		if err != nil || appImg.Bounds() != image.Rect(0, 0, 1024, 1024) {
			t.Fatal("package lost standard app tile", err)
		}
	}
	again := filepath.Join(t.TempDir(), "frozen-assets")
	next, err := Prepare(filepath.Join(out, "profile.json"), filepath.Join(out, "logo.png"), again)
	if err != nil || next.Brand.LogoAsset != p.Brand.LogoAsset {
		t.Fatal("repackaging a frozen profile changes its source identity", err)
	}
	for _, file := range []string{"logo-light.png", "logo-dark.png", "app-light.png", "app-dark.png", "enterprise.icns", "enterprise-dark.icns"} {
		a, _ := os.ReadFile(filepath.Join(out, file))
		b, _ := os.ReadFile(filepath.Join(again, file))
		if !bytes.Equal(a, b) {
			t.Fatalf("upload/repackaging standard drifted: %s", file)
		}
	}
}

func TestEnterpriseProfileRejectsTrailingJSON(t *testing.T) {
	_, profile, _ := fixture(t)
	b, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	for _, tail := range []string{" {}", " invalid"} {
		if err := os.WriteFile(profile, append(bytes.Clone(b), []byte(tail)...), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readProfile(profile); err == nil {
			t.Fatal("accepted multiple or damaged profile values")
		}
	}
}

func TestEnterpriseManifestIntegrityAndUnsignedRefusal(t *testing.T) {
	p, _, _ := fixture(t)
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p.Brand.LogoAsset = distribution.AssetID([]byte("logo"))
	p.Updates.Mode = "platform"
	p.Updates.Channel = "stable"
	p.Updates.FeedURL = "https://distribution.invalid/manifest.json"
	p.Updates.ManifestKeys = map[string]string{"key-1": base64.StdEncoding.EncodeToString(pub)}
	dir := t.TempDir()
	name := "acme-assistant-windows-amd64.exe"
	data := []byte("signed fixture, not a real executable")
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	result := Result{Profile: p, Version: "1.0.0", Signed: true, Target: "windows/amd64", Assets: map[string]distribution.Asset{name: {Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}}}
	m, err := SignManifest(result, dir, "https://distribution.invalid/packages", "key-1", 1, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := distribution.VerifyManifest(m, p); err != nil {
		t.Fatal(err)
	}
	m.DistributionID = "other-tenant"
	if err := distribution.VerifyManifest(m, p); err == nil {
		t.Fatal("accepted another enterprise")
	}
	result.Signed = false
	if _, err := SignManifest(result, dir, "https://distribution.invalid/packages", "key-1", 2, key); err == nil {
		t.Fatal("unsigned artifacts were publishable")
	}
	result.Signed = true
	if err := os.WriteFile(filepath.Join(dir, name), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SignManifest(result, dir, "https://distribution.invalid/packages", "key-1", 2, key); err == nil {
		t.Fatal("modified artifact was publishable")
	}
}
