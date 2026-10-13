package gui

import (
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/distribution"
)

func TestEnterpriseInstanceTargetsOwnBundleAndHoldsIndependentLock(t *testing.T) {
	p := distribution.Profile{DistributionID: "msai-client"}
	p.Identity.MacBundleID = "com.msai.client.test"
	if got := instanceBundleID(p); got != p.Identity.MacBundleID {
		t.Fatal("second enterprise launch activates public Magpie", got)
	}
	if got := instanceBundleID(distribution.Profile{}); got != "com.yetone.magpie" {
		t.Fatal("public activation changed", got)
	}
	r := t.TempDir()
	public := instanceLock(filepath.Join(r, "magpie"), "/Applications/magpie.app/Contents/MacOS/magpie")
	enterprise := instanceLock(filepath.Join(r, "msai-client"), "/Applications/msai-client.app/Contents/MacOS/msai-client")
	old := heldInstance
	t.Cleanup(func() { heldInstance = old })
	if ok, err := holdInstance(public); !ok || err != nil {
		t.Fatal(ok, err)
	}
	a := heldInstance
	t.Cleanup(func() { a.Close() })
	if ok, err := holdInstance(enterprise); !ok || err != nil {
		t.Fatal("enterprise contends with public lock", ok, err)
	}
	b := heldInstance
	t.Cleanup(func() { b.Close() })
	if ok, err := holdInstance(public); ok || err != nil {
		t.Fatal("public single-instance guard lost", ok, err)
	}
	if ok, err := holdInstance(enterprise); ok || err != nil {
		t.Fatal("enterprise single-instance guard lost", ok, err)
	}
}
