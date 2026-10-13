package update

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/distribution"
)

func TestEnterpriseUpdateTrustReplayAndCorruptSequence(t *testing.T) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p := distribution.Profile{DistributionID: "acme-assistant"}
	p.Identity.ArtifactSlug = "acme-assistant"
	p.Updates.Channel = "stable"
	p.Updates.ManifestKeys = map[string]string{"release": base64.StdEncoding.EncodeToString(pub)}
	m := distribution.Manifest{SchemaVersion: 1, DistributionID: p.DistributionID, Channel: "stable", Sequence: 2, Version: "1.2.3", ProfileRevision: 1, BundleName: "acme-assistant.app", KeyID: "release", Assets: map[string]distribution.Asset{"acme-assistant-darwin-arm64.zip": {URL: "https://downloads.invalid/app.zip", Size: 123, SHA256: strings.Repeat("a", 64)}}}
	encode := func(m distribution.Manifest) []byte {
		t.Helper()
		payload, err := m.SigningBytes()
		if err != nil {
			t.Fatal(err)
		}
		m.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, payload))
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	path := filepath.Join(t.TempDir(), "state", "sequence")
	good := encode(m)
	if r, err := enterpriseReleaseFor(bytes.NewReader(good), p, path); err != nil || r.Version != m.Version {
		t.Fatalf("signed update: %v %v", r, err)
	}
	for _, body := range [][]byte{append(bytes.Clone(good), []byte(" {}")...), bytes.Replace(good, []byte("1.2.3"), []byte("1.2.4"), 1)} {
		if _, err := enterpriseReleaseFor(bytes.NewReader(body), p, path); err == nil {
			t.Fatal("accepted unsigned changes or trailing JSON")
		}
	}
	m.Sequence = 1
	if _, err := enterpriseReleaseFor(bytes.NewReader(encode(m)), p, path); err == nil {
		t.Fatal("accepted a replayed update")
	}
	m.Sequence = 3
	m.DistributionID = "other-enterprise"
	if _, err := enterpriseReleaseFor(bytes.NewReader(encode(m)), p, path); err == nil {
		t.Fatal("accepted another enterprise's signed update")
	}
	m.DistributionID = p.DistributionID
	if err := os.WriteFile(path, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := enterpriseReleaseFor(bytes.NewReader(encode(m)), p, path); err == nil {
		t.Fatal("treated an unreadable update sequence as zero")
	}
}

func TestEnterpriseArtifactsIgnorePublicGitHubMirrors(t *testing.T) {
	asset := "https://github.com/company/releases/download/v1.0.0/company.zip"
	mirror := "https://mirror.invalid/"
	if got := artifactURL("acme-assistant", mirror, asset); got != asset {
		t.Fatalf("enterprise artifact was redirected through a public mirror: %s", got)
	}
	if got := artifactURL("", mirror, asset); got != mirror+asset {
		t.Fatalf("generic mirror behavior changed: %s", got)
	}
}
