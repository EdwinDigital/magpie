package enterprisedistribution

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/distribution"
	"github.com/yetone/magpie/internal/enterprisebuild"
)

func request(t *testing.T, s http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&data).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, &data)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, r)
	return rec
}

func TestEnterpriseProfileBindsUpdatesToItsDownloadChannel(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 32)
	cfg := Config{Store: t.TempDir(), BaseURL: "https://distribution.invalid", Principals: []Principal{{TenantID: "acme", Role: "admin", Token: token}}}
	s, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	p := distribution.Profile{SchemaVersion: 1, TenantID: "acme", DistributionID: "acme-assistant", ProfileRevision: 1}
	p.Brand.DisplayName = "Acme"
	p.Identity.MacBundleID, p.Identity.WindowsAppID = "com.acme.assistant", "com.acme.assistant"
	p.Identity.ArtifactSlug, p.Identity.URLScheme = "acme-assistant", "acme-assistant"
	p.Updates.Mode, p.Updates.Channel = "platform", "stable"
	p.Updates.ManifestKeys = map[string]string{"release": base64.StdEncoding.EncodeToString(pub)}
	var logo bytes.Buffer
	mark := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	mark.SetNRGBA(8, 8, color.NRGBA{A: 255})
	if err := png.Encode(&logo, mark); err != nil {
		t.Fatal(err)
	}
	for _, feed := range []string{"https://usemagpie.ai/api/latest", "https://distribution.invalid/v1/updates/other/acme-assistant/stable/manifest.json", "https://distribution.invalid/v1/updates/acme/acme-assistant/beta/manifest.json"} {
		p.Updates.FeedURL = feed
		got := request(t, s, "POST", "/v1/profiles", token, map[string]any{"profile": p, "logoBase64": base64.StdEncoding.EncodeToString(logo.Bytes())})
		if got.Code != 400 {
			t.Fatalf("accepted a different distribution channel %s: %d %s", feed, got.Code, got.Body)
		}
	}
	p.Updates.FeedURL = ""
	got := request(t, s, "POST", "/v1/profiles", token, map[string]any{"profile": p, "logoBase64": base64.StdEncoding.EncodeToString(logo.Bytes())})
	var saved distribution.Profile
	if got.Code != 201 || json.Unmarshal(got.Body.Bytes(), &saved) != nil || saved.Updates.FeedURL != cfg.BaseURL+"/v1/updates/acme/acme-assistant/stable/manifest.json" {
		t.Fatalf("download channel was not embedded: %d %s", got.Code, got.Body)
	}
}

func TestEnterprisePublicationTrustMergeAndRecovery(t *testing.T) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 32)
	cfg := Config{Store: t.TempDir(), BaseURL: "https://distribution.invalid", Principals: []Principal{{TenantID: "acme", Role: "admin", Token: token}}, SigningKey: key, SigningKeyID: "release"}
	s, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	p := distribution.Profile{SchemaVersion: 1, TenantID: "acme", DistributionID: "acme-assistant", ProfileRevision: 1}
	p.Brand.DisplayName, p.Brand.LogoAsset = "Acme", distribution.AssetID([]byte("logo"))
	p.Identity.MacBundleID, p.Identity.WindowsAppID = "com.acme.assistant", "com.acme.assistant"
	p.Identity.ArtifactSlug, p.Identity.URLScheme = "acme-assistant", "acme-assistant"
	p.Updates.Mode, p.Updates.Channel = "platform", "stable"
	p.Updates.FeedURL = cfg.BaseURL + "/v1/updates/acme/acme-assistant/stable/manifest.json"
	p.Updates.ManifestKeys = map[string]string{"release": base64.StdEncoding.EncodeToString(pub)}
	ready := func(target string) Job {
		t.Helper()
		j := Job{ID: randomID(), TenantID: "acme", DistributionID: p.DistributionID, ProfileRevision: 1, Status: "ready", Version: "1.0.0", BuildNumber: 1, Target: target}
		name := "acme-assistant-" + strings.ReplaceAll(target, "/", "-") + ".zip"
		data := []byte("fixture only: " + target)
		sum := sha256.Sum256(data)
		j.Result = &enterprisebuild.Result{Profile: p, Version: j.Version, BuildNumber: 1, Target: target, Signed: true, Assets: map[string]distribution.Asset{name: {Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}}}
		dir := filepath.Join(cfg.Store, "jobs", j.ID)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
		s.mu.Lock()
		s.jobs[j.ID] = j
		s.mu.Unlock()
		if err := s.saveJob(j); err != nil {
			t.Fatal(err)
		}
		return j
	}
	arm := ready("darwin/arm64")
	arm.Result.Dirty = true
	if got := request(t, s, "POST", "/v1/builds/"+arm.ID+"/publish", token, nil); got.Code != 409 {
		t.Fatal("published a dirty build")
	}
	arm.Result.Dirty, arm.Result.Signed = false, false
	if got := request(t, s, "POST", "/v1/builds/"+arm.ID+"/publish", token, nil); got.Code != 409 {
		t.Fatal("published an unsigned build")
	}
	arm.Result.Signed = true
	for _, j := range []Job{arm, ready("darwin/amd64")} {
		if got := request(t, s, "POST", "/v1/builds/"+j.ID+"/publish", token, nil); got.Code != 201 {
			t.Fatalf("publish: %d %s", got.Code, got.Body)
		}
	}
	feed := request(t, s, "GET", "/v1/updates/acme/acme-assistant/stable/manifest.json", token, nil)
	var manifest distribution.Manifest
	if feed.Code != 200 || json.Unmarshal(feed.Body.Bytes(), &manifest) != nil || len(manifest.Assets) != 2 || manifest.Sequence != 2 {
		t.Fatalf("multi-target feed lost an asset: %s", feed.Body)
	}
	if err := distribution.VerifyManifest(manifest, p); err != nil {
		t.Fatal(err)
	}
	tampered := ready("darwin/arm64")
	tampered.Result.Version, tampered.Version = "1.0.1", "1.0.1"
	for name := range tampered.Result.Assets {
		if err := os.WriteFile(filepath.Join(cfg.Store, "jobs", tampered.ID, name), []byte("tampered"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s.mu.Lock()
	s.jobs[tampered.ID] = tampered
	s.mu.Unlock()
	if got := request(t, s, "POST", "/v1/builds/"+tampered.ID+"/publish", token, nil); got.Code != 409 {
		t.Fatal("published altered artifact bytes")
	}
	interrupted := Job{ID: randomID(), TenantID: "acme", DistributionID: p.DistributionID, Status: "building"}
	if err := s.saveJob(interrupted); err != nil {
		t.Fatal(err)
	}
	s.Close()
	resumed, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	if resumed.jobs[interrupted.ID].Status != "failed" {
		t.Fatal("interrupted signing/build job was silently retried")
	}
	if got := request(t, resumed, "GET", "/v1/downloads", token, nil); got.Code != 200 || !strings.Contains(got.Body.String(), "darwin-arm64") || !strings.Contains(got.Body.String(), "darwin-amd64") {
		t.Fatalf("published downloads did not survive worker restart: %s", got.Body)
	}
}

func TestEnterpriseDistributionTenantLifecycle(t *testing.T) {
	admin, employee, other := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)
	cfg := Config{Store: t.TempDir(), BaseURL: "https://distribution.invalid", Principals: []Principal{{TenantID: "acme", Role: "admin", Token: admin}, {TenantID: "acme", Role: "employee", Token: employee}, {TenantID: "other", Role: "employee", Token: other}}}
	cfg.Build = func(ctx context.Context, o enterprisebuild.Options) (enterprisebuild.Result, error) {
		var p distribution.Profile
		b, err := os.ReadFile(o.ProfilePath)
		if err != nil {
			return enterprisebuild.Result{}, err
		}
		if err := json.Unmarshal(b, &p); err != nil {
			return enterprisebuild.Result{}, err
		}
		if err := os.MkdirAll(o.Output, 0o700); err != nil {
			return enterprisebuild.Result{}, err
		}
		data := []byte("test fixture only")
		name := "acme-assistant-windows-amd64.exe"
		if err := os.WriteFile(filepath.Join(o.Output, name), data, 0o600); err != nil {
			return enterprisebuild.Result{}, err
		}
		sum := sha256.Sum256(data)
		return enterprisebuild.Result{Profile: p, Version: o.Version, BuildNumber: o.BuildNumber, Target: o.Target, Signed: true, Assets: map[string]distribution.Asset{name: {Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}}}, nil
	}
	s, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := request(t, s, "GET", "/v1/downloads", "", nil); got.Code != 401 {
		t.Fatal("anonymous employee enumeration")
	}
	if got := request(t, s, "POST", "/v1/profiles", employee, map[string]string{}); got.Code != 403 {
		t.Fatal("employee can configure builds")
	}
	p := distribution.Profile{SchemaVersion: 1, TenantID: "acme", DistributionID: "acme-assistant", ProfileRevision: 1}
	p.Brand.DisplayName = "Acme"
	p.Identity.MacBundleID = "com.acme.assistant"
	p.Identity.WindowsAppID = "com.acme.assistant"
	p.Identity.ArtifactSlug = "acme-assistant"
	p.Identity.URLScheme = "acme-assistant"
	p.Updates.Mode = "manual"
	var logo bytes.Buffer
	mark := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	mark.SetNRGBA(8, 8, color.NRGBA{A: 255})
	_ = png.Encode(&logo, mark)
	body := map[string]any{"profile": p, "logoBase64": base64.StdEncoding.EncodeToString(logo.Bytes())}
	if got := request(t, s, "POST", "/v1/profiles", admin, body); got.Code != 201 {
		t.Fatalf("profile: %d %s", got.Code, got.Body)
	}
	if got := request(t, s, "POST", "/v1/profiles", admin, body); got.Code != 409 {
		t.Fatal("profile revision was overwritten")
	}
	if got := request(t, s, "GET", "/v1/profiles/acme-assistant/1", other, nil); got.Code != 404 {
		t.Fatal("another enterprise reads the profile")
	}
	got := request(t, s, "POST", "/v1/builds", admin, map[string]any{"distributionId": "acme-assistant", "profileRevision": 1, "version": "1.0.0", "buildNumber": 1, "target": "windows/amd64"})
	var job Job
	if got.Code != 202 || json.Unmarshal(got.Body.Bytes(), &job) != nil {
		t.Fatalf("queue: %s", got.Body)
	}
	deadline := time.After(5 * time.Second)
	for {
		read := request(t, s, "GET", "/v1/builds/"+job.ID, admin, nil)
		_ = json.Unmarshal(read.Body.Bytes(), &job)
		if job.Status == "ready" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("worker did not finish")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if got := request(t, s, "GET", "/v1/builds/"+job.ID, other, nil); got.Code != 404 {
		t.Fatal("cross-tenant build read")
	}
	if got := request(t, s, "POST", "/v1/builds/"+job.ID+"/publish", admin, nil); got.Code != 201 {
		t.Fatalf("publish: %s", got.Body)
	}
	if got := request(t, s, "POST", "/v1/builds/"+job.ID+"/publish", admin, nil); got.Code != 409 {
		t.Fatal("published artifact was overwritten")
	}
	if got := request(t, s, "GET", "/v1/artifacts/acme/acme-assistant/1.0.0/windows-amd64/%2e%2e", employee, nil); got.Code != 404 {
		t.Fatalf("artifact route allowed a parent directory: %d", got.Code)
	}
	if got := request(t, s, "GET", "/v1/downloads", other, nil); got.Body.String() != "[]\n" {
		t.Fatalf("cross-tenant download list: %s", got.Body)
	}
	list := request(t, s, "GET", "/v1/downloads", employee, nil)
	var items []struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(list.Body.Bytes(), &items) != nil || len(items) != 1 {
		t.Fatalf("employee list: %s", list.Body)
	}
	link := strings.TrimPrefix(items[0].URL, cfg.BaseURL)
	if got := request(t, s, "GET", link, "", nil); got.Code != 200 || got.Body.String() != "test fixture only" {
		t.Fatalf("download capability: %d %s", got.Code, got.Body)
	}
	s.mu.Lock()
	ticketID := strings.TrimPrefix(link, "/download/")
	expired := s.tickets[ticketID]
	expired.expiry = time.Now().Add(-time.Minute)
	s.tickets[ticketID] = expired
	s.mu.Unlock()
	if got := request(t, s, "GET", link, "", nil); got.Code != 404 {
		t.Fatal("expired download grant is usable")
	}
	if got := request(t, s, "POST", "/v1/builds", admin, map[string]any{"distributionId": "acme-assistant", "profileRevision": 1, "version": "1.0.0", "buildNumber": 1, "target": "windows/amd64", "source": "https://evil.invalid"}); got.Code != 400 {
		t.Fatal("caller can select arbitrary worker source")
	}
}
