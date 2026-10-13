package distribution

import "testing"

func TestWiringIdentityIsPackageStableAndDoesNotPrefixAnotherToken(t *testing.T) {
	old := current
	t.Cleanup(func() { current = old })
	current = Profile{}
	if WiringID() != "magpie" || WiringToken() != "magpie" {
		t.Fatal("public identity changed")
	}
	current.DistributionID = "acme"
	id, token := WiringID(), WiringToken()
	current.Brand.DisplayName = "Renamed by user"
	if WiringID() != id || WiringToken() != token {
		t.Fatal("display branding changed technical ownership")
	}
	current.DistributionID = "acme-client"
	if WiringID() == id || WiringToken() == token || WiringToken()[:len(token)] == token {
		t.Fatal("enterprise tokens collide or have ambiguous prefixes")
	}
}
