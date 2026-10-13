package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/distribution"
)

type channelTransport func(*http.Request) (*http.Response, error)

func (f channelTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEnterpriseFeedNeverUsesPublicOverridesOrFallback(t *testing.T) {
	t.Setenv("MAGPIE_UPDATE_FEED", "https://public-test.invalid/api/latest")
	p := distribution.Profile{DistributionID: "acme-assistant"}
	p.Updates.Mode, p.Updates.Channel = "platform", "stable"
	p.Updates.FeedURL = "https://company.invalid/v1/updates/acme/acme-assistant/stable/manifest.json"
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p.Identity.ArtifactSlug = p.DistributionID
	p.Updates.ManifestKeys = map[string]string{"release": base64.StdEncoding.EncodeToString(pub)}
	m := distribution.Manifest{SchemaVersion: 1, DistributionID: p.DistributionID, Channel: "stable", Sequence: 1, Version: "1.2.0", ProfileRevision: 1, BundleName: "acme-assistant.app", KeyID: "release", Assets: map[string]distribution.Asset{"acme-assistant-darwin-arm64.zip": {URL: "https://company.invalid/app.zip", Size: 123, SHA256: strings.Repeat("a", 64)}}}
	signed := func(m distribution.Manifest) string {
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
		return string(b)
	}
	old := client
	t.Cleanup(func() { client = old })
	for _, tc := range []struct {
		name   string
		status int
		body   string
		wantOK bool
	}{
		{"signed enterprise version", 200, signed(m), true},
		{"channel unavailable", 503, "unavailable", false},
		{"employee credential expired", 401, "expired", false},
		{"public unsigned release", 200, `{"version":"99.0.0","assets":{}}`, false},
		{"other signed channel", 200, func() string { m.Channel = "beta"; return signed(m) }(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var asked []string
			client = &http.Client{Transport: channelTransport(func(r *http.Request) (*http.Response, error) {
				asked = append(asked, r.URL.String())
				return &http.Response{StatusCode: tc.status, Status: http.StatusText(tc.status), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			r, err := latestInFor(context.Background(), "zh", p, filepath.Join(t.TempDir(), "sequence"))
			if (err == nil) != tc.wantOK {
				t.Fatalf("enterprise check: %v %v", r, err)
			}
			if len(asked) != 1 || asked[0] != p.Updates.FeedURL+"?lang=zh" {
				t.Fatalf("enterprise detection left its download channel: %v", asked)
			}
		})
	}
	p.Updates.Mode = "manual"
	client = &http.Client{Transport: channelTransport(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("manual package made an update request: %s", r.URL)
		return nil, nil
	})}
	if _, err := latestInFor(context.Background(), "zh", p, filepath.Join(t.TempDir(), "sequence")); err == nil || policyFor(p).FeedURL != "" {
		t.Fatal("manual package acquired an update feed")
	}
	if got := policyFor(distribution.Profile{}); got.Mode != "public" || got.FeedURL != "https://public-test.invalid/api/latest" {
		t.Fatalf("public package's existing test feed changed: %+v", got)
	}
}
