package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/distribution"
)

func TestEnterpriseServiceNeverUsesPublicRecords(t *testing.T) {
	for _, platform := range []string{"darwin", "linux"} {
		t.Run(platform, func(t *testing.T) {
			fakes(t, platform)
			prev := packageProfile
			p := distribution.Profile{DistributionID: "corp"}
			p.Identity.ArtifactSlug = "msai-client"
			p.Identity.MacBundleID = "com.msai.client.test"
			packageProfile = func() distribution.Profile { return p }
			t.Cleanup(func() { packageProfile = prev })
			public := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "systemd", "user", "magpie.service")
			if platform == "darwin" {
				public = filepath.Join(home(), "Library", "LaunchAgents", Label+".plist")
			}
			if err := os.MkdirAll(filepath.Dir(public), 0o700); err != nil {
				t.Fatal(err)
			}
			write(t, public, "public record stays unchanged")
			if err := Install(Options{}); err != nil {
				t.Fatal(err)
			}
			m, err := current()
			if err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(m.file())
			if err != nil {
				t.Fatal(err)
			}
			want := "msai-client.service"
			if platform == "darwin" {
				want = "com.msai.client.test.serve.plist"
				if !strings.Contains(string(b), "<string>com.msai.client.test.serve</string>") || !strings.Contains(logFile(), filepath.Join("Logs", "msai-client")) {
					t.Fatal("enterprise launch identity or logs are public", string(b), logFile())
				}
			}
			if filepath.Base(m.file()) != want {
				t.Fatal("wrong enterprise service file", m.file())
			}
			if err := Uninstall(); err != nil {
				t.Fatal(err)
			}
			if b, err := os.ReadFile(public); err != nil || string(b) != "public record stays unchanged" {
				t.Fatal("enterprise changed public service", err, string(b))
			}
		})
	}
}
