package distribution

import "testing"

func testProfile() Profile {
	p := Profile{SchemaVersion: 1, TenantID: "acme", DistributionID: "acme-assistant", ProfileRevision: 1}
	p.Brand.DisplayName = "Acme Assistant"
	p.Brand.LogoAsset = AssetID([]byte("logo"))
	p.Identity.MacBundleID = "com.acme.assistant"
	p.Identity.WindowsAppID = "com.acme.assistant"
	p.Identity.ArtifactSlug = "acme-assistant"
	p.Identity.URLScheme = "acme-assistant"
	p.Updates.Mode = "manual"
	return p
}

func TestEnterpriseProfileValidation(t *testing.T) {
	p := testProfile()
	if err := Validate(p); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Profile){func(p *Profile) { p.Identity.ArtifactSlug = "../escape" }, func(p *Profile) { p.Brand.DisplayName = "Acme\n" }, func(p *Profile) { p.Updates.Mode = "upstream" }, func(p *Profile) { p.Updates.Mode = "platform" }, func(p *Profile) { p.EnterpriseDefaults.VisibleProviders = []string{"custom:../private"} }} {
		next := p
		mutate(&next)
		if err := Validate(next); err == nil {
			t.Fatalf("accepted %+v", next)
		}
	}
}
