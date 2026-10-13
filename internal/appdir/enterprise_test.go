package appdir

import (
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/distribution"
)

func TestEnterprisePathsMatchCLIAndNeverPublicState(t *testing.T) {
	p := distribution.Profile{DistributionID: "msai-client"}
	p.Identity.ArtifactSlug = "msai-client"
	for _, base := range []string{filepath.Join("home", ".config", "magpie"), filepath.Join("home", ".cache", "magpie"), filepath.Join("home", "Library", "Caches", "magpie")} {
		want := filepath.Join(filepath.Dir(base), "msai-client")
		if got := distributionDir(base, p); got != want || got == base {
			t.Fatalf("%s => %s want %s", base, got, want)
		}
		if got := distributionDir(base, distribution.Profile{}); got != base {
			t.Fatal("public paths changed")
		}
	}
	base := filepath.Join("download", "data")
	if got := distributionDir(base, p); got != filepath.Join(base, "msai-client") {
		t.Fatal("portable files not isolated", got)
	}
	p.DistributionID = "other-tenant"
	if got := distributionDir(base, p); got == filepath.Join(base, "msai-client") {
		t.Fatal("another distribution shares state")
	}
	p.Brand.DisplayName = "Renamed locally"
	if got := distributionDir(base, p); got != filepath.Join(base, "msai-client", "enterprises", "other-tenant") {
		t.Fatal("display branding changed storage")
	}
}
