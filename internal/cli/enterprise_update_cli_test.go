package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/update"
)

func TestEnterpriseCLIUpdateAdviceStaysOnDownloadChannel(t *testing.T) {
	old := updatePackagePolicy
	policy := update.Policy{DistributionID: "acme-assistant", Mode: "platform"}
	updatePackagePolicy = func() update.Policy { return policy }
	t.Cleanup(func() { updatePackagePolicy = old })
	if err := updateMirrorCmd([]string{"https://public-mirror.invalid/"}); err == nil || !strings.Contains(err.Error(), "distribution channel") {
		t.Fatalf("enterprise CLI offered a public update mirror: %v", err)
	}
	if got := via(update.WithMirror(context.Background(), "https://public-mirror.invalid/")); got != "" {
		t.Fatalf("enterprise CLI claimed it would use a public mirror: %s", got)
	}
	for _, url := range []string{"", "https://company.invalid/releases/1.2.0"} {
		got := updateDownloadSource(policy, &update.Release{URL: url})
		if strings.Contains(got, update.Site) || url != "" && got != url {
			t.Fatalf("enterprise advice pointed outside its distribution channel: %s", got)
		}
	}
	policy.Mode = "manual"
	if err := updateAutoCmd([]string{"on"}); err == nil || !strings.Contains(err.Error(), "enterprise distribution channel") {
		t.Fatalf("manual enterprise CLI enabled automatic checks: %v", err)
	}
	if got := updateDownloadSource(update.Policy{Mode: "public"}, &update.Release{}); got != update.Site {
		t.Fatalf("public CLI advice changed: %s", got)
	}
}
