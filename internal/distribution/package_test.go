package distribution

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"testing"
)

func packageFixture(t *testing.T) Package {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	p := testProfile()
	p.Brand.LogoAsset = AssetID(b.Bytes())
	return Package{SchemaVersion: 1, Profile: p, Version: "1.2.3", BuildNumber: 9, RuntimeVersion: "0.1.1172", Target: "darwin/arm64", Logo: b.Bytes()}
}

func TestEnterprisePackageRejectsCorruptOrPublicFallback(t *testing.T) {
	c := packageFixture(t)
	encode := func(c Package) []byte {
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if _, err := DecodePackage(encode(c)); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Package){func(c *Package) { c.Profile.DistributionID = "" }, func(c *Package) { c.Logo[0] = 0 }, func(c *Package) { c.Version = "dev" }, func(c *Package) { c.Target = "linux/arm64" }, func(c *Package) { c.SchemaVersion = 2 }, func(c *Package) { c.BuildNumber = 65536 }} {
		next := packageFixture(t)
		change(&next)
		if _, err := DecodePackage(encode(next)); err == nil {
			t.Fatal("accepted damaged enterprise package")
		}
	}
	for _, b := range [][]byte{nil, []byte("{}"), append(encode(c), []byte(" {}")...), bytes.Repeat([]byte(" "), MaxPackageSize+1)} {
		if _, err := DecodePackage(b); err == nil {
			t.Fatal("accepted invalid package resource")
		}
	}
}

func TestEnterpriseExecutableAndHelpUsePackageIdentity(t *testing.T) {
	old, oldMode, oldConfig := current, resourcePackage, packageConfig
	t.Cleanup(func() { current, resourcePackage, packageConfig = old, oldMode, oldConfig })
	current = packageFixture(t).Profile
	current.Brand.DisplayName = "Company Assistant"
	if ExecutableName() != "acme-assistant" {
		t.Fatal("process name follows a public/display name")
	}
	if got := CLIHelp("magpie — client\n  magpie status\n  magpie version"); got != "Company Assistant — client\n  acme-assistant status\n  acme-assistant version" {
		t.Fatal(got)
	}
	resourcePackage = "1"
	packageConfig = packageFixture(t)
	if PackageVersion("public") != "1.2.3" {
		t.Fatal("enterprise checks public version")
	}
	logo := Logo()
	logo[0] = 0
	if Logo()[0] == 0 {
		t.Fatal("caller changed immutable package logo")
	}
}
