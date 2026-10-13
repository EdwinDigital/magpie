package gui

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/settings"
)

func enterpriseRequest(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&data).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, &data)
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestEnterpriseCandidatesCoverBuiltinAddDirectory(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	source, err := os.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	_, subs, ok := strings.Cut(string(source), "const SUBS = [")
	if !ok {
		t.Fatal("subscription directory was not found")
	}
	subs, _, _ = strings.Cut(subs, "];")
	have := map[string]bool{}
	for _, c := range enterpriseProviders() {
		have[c.ID] = true
	}
	for _, m := range regexp.MustCompile(`agent:\s*"([^"]+)"`).FindAllStringSubmatch(subs, -1) {
		if !have["subscription:"+m[1]] {
			t.Errorf("built-in %q cannot be selected in enterprise settings", m[1])
		}
	}
}

func TestEnterpriseHandlerSaveBootConflictAndGuards(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	h := Handler(nil, nil)
	initial := enterpriseState()
	for _, path := range []string{"/api/library", "/api/library/market/skills", "/api/library/servers/save", "/api/plugins/listings", "/api/plugins/github", "/api/plugins/market", "/api/plugins/search?q=secret", "/api/plugins/npm?names=not-installed", "/api/plugins/page?name=not-installed"} {
		for _, method := range []string{"GET", "POST"} {
			if got := enterpriseRequest(t, h, method, path, nil); got.Code != 403 || !strings.Contains(got.Body.String(), "feature_disabled") {
				t.Fatalf("%s %s: %d %s", method, path, got.Code, got.Body)
			}
		}
	}
	next := initial.Effective
	next.AppName = "Acme <Assistant>"
	next.OverrideFields = []string{"appName", "libraryEnabled", "publicPluginCatalog"}
	next.LibraryEnabled = true
	next.PublicPluginCatalog = true
	got := enterpriseRequest(t, h, "POST", "/api/enterprise", map[string]any{"config": next, "revision": initial.Revision})
	if got.Code != 200 {
		t.Fatalf("save: %s", got.Body)
	}
	boot := enterpriseRequest(t, h, "GET", "/boot.js", nil).Body.String()
	if !strings.Contains(boot, `Acme \u003cAssistant\u003e`) {
		t.Fatalf("boot brand was not JSON escaped: %s", boot)
	}
	if got := enterpriseRequest(t, h, "POST", "/api/enterprise", map[string]any{"config": next, "revision": initial.Revision}); got.Code != 409 {
		t.Fatal("missing conflict detection")
	}
	if got := enterpriseRequest(t, h, "POST", "/api/application/restart", nil); got.Code != 403 {
		t.Fatal("a browser can restart the server")
	}
	if got := enterpriseRequest(t, h, "GET", "/api/enterprise/providers", nil); got.Code != 200 || !strings.Contains(got.Body.String(), "subscription:copilot") || !strings.Contains(got.Body.String(), "preset:ollama") {
		t.Fatalf("complete candidate list: %s", got.Body)
	}
}

func TestEnterpriseLogoValidationAndRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	h := Handler(nil, nil)
	var valid bytes.Buffer
	mark := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	for y := 2; y < 8; y++ {
		for x := 2; x < 8; x++ {
			mark.Set(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	_ = png.Encode(&valid, mark)
	for _, data := range [][]byte{[]byte("<svg onload='alert(1)'/>"), valid.Bytes()} {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, _ := writer.CreateFormFile("logo", "logo.png")
		_, _ = part.Write(data)
		_ = writer.Close()
		r := httptest.NewRequest("POST", "/api/enterprise/logo", &body)
		r.Header.Set("Content-Type", writer.FormDataContentType())
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if bytes.Equal(data, valid.Bytes()) {
			if rec.Code != 200 {
				t.Fatalf("upload: %s", rec.Body)
			}
			var asset struct{ Asset, URL string }
			_ = json.Unmarshal(rec.Body.Bytes(), &asset)
			round := enterpriseRequest(t, h, "GET", asset.URL, nil)
			if round.Code != 200 || round.Header().Get("Content-Type") != "image/png" {
				t.Fatal("uploaded logo is not readable")
			}
			cfg := enterpriseState()
			e := cfg.Effective
			e.LogoAsset = asset.Asset
			e.OverrideFields = []string{"logoAsset"}
			if got := enterpriseRequest(t, h, "POST", "/api/enterprise", map[string]any{"config": e, "revision": cfg.Revision}); got.Code != 200 {
				t.Fatalf("logo save: %s", got.Body)
			}
		} else if rec.Code != 400 {
			t.Fatal("accepted executable SVG")
		}
	}
	if got := enterpriseRequest(t, h, "GET", "/api/enterprise/logo/not-a-hash.png", nil); got.Code != 404 {
		t.Fatal("accepted an arbitrary logo path")
	}
	if err := settings.SetEnterprise(nil, settings.EnterpriseRevision(settings.Load())); err != nil {
		t.Fatal(err)
	}
}

func TestEnterpriseWhiteUploadGeneratesReadableThemeAssets(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	h := Handler(nil, nil)
	b, err := os.ReadFile("../enterprisebuild/testdata/microsoft-white-transparent.png")
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("logo", "white.png")
	_, _ = part.Write(b)
	_ = w.Close()
	r := httptest.NewRequest("POST", "/api/enterprise/logo", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	var reply struct {
		Asset    string
		LogoURLs map[string]string `json:"logoUrls"`
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &reply) != nil {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	for _, mode := range []string{"light", "dark"} {
		got := enterpriseRequest(t, h, "GET", reply.LogoURLs[mode], nil)
		img, err := png.Decode(bytes.NewReader(got.Body.Bytes()))
		if got.Code != 200 || err != nil {
			t.Fatalf("%s icon: %d %v", mode, got.Code, err)
		}
		c := color.NRGBAModel.Convert(img.At(300, 300)).(color.NRGBA)
		if mode == "light" && c.R > 30 || mode == "dark" && c.R < 240 || c.A != 255 {
			t.Fatalf("%s contrast: %v", mode, c)
		}
		_, _, _, a := img.At(512, 512).RGBA()
		if a != 0 || img.Bounds() != image.Rect(0, 0, 1024, 1024) {
			t.Fatal("theme asset lost standard geometry or transparent gaps")
		}
	}
	state := enterpriseState()
	e := state.Effective
	e.LogoAsset = reply.Asset
	e.OverrideFields = []string{"logoAsset"}
	if got := enterpriseRequest(t, h, "POST", "/api/enterprise", map[string]any{"config": e, "revision": state.Revision}); got.Code != 200 {
		t.Fatal(got.Body.String())
	}
	if enterpriseState().LogoURLs["dark"] != reply.LogoURLs["dark"] {
		t.Fatal("save lost theme variants")
	}
	if got := enterpriseRequest(t, h, "GET", strings.Split(reply.LogoURLs["light"], "?")[0]+"?theme=invalid", nil); got.Code != 404 {
		t.Fatal("accepted invalid image variant")
	}
}

func TestEnterpriseRejectsTrailingJSONWithoutSaving(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	h := Handler(nil, nil)
	initial := enterpriseState()
	config := initial.Effective
	config.AppName = "Must not save"
	config.OverrideFields = []string{"appName"}
	b, err := json.Marshal(map[string]any{"config": config, "revision": initial.Revision})
	if err != nil {
		t.Fatal(err)
	}
	for _, tail := range []string{" {}", " garbage"} {
		r := httptest.NewRequest("POST", "/api/enterprise", strings.NewReader(string(b)+tail))
		r.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != 400 || enterpriseState().Revision != initial.Revision {
			t.Fatalf("trailing input changed settings: %d %s", rec.Code, rec.Body)
		}
	}
}

func TestEnterpriseRestartStateFollowsLaunchBrand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	old := enterprisePendingRestart
	t.Cleanup(func() { enterprisePendingRestart = old })
	launch := enterpriseState().Effective
	enterprisePendingRestart = func(current settings.Enterprise) []string {
		if current.AppName != launch.AppName {
			return []string{"Native application menu entries"}
		}
		return []string{}
	}
	next := launch
	next.AppName = "Acme Runtime"
	next.OverrideFields = []string{"appName"}
	if err := settings.SetEnterprise(&next, enterpriseState().Revision); err != nil {
		t.Fatal(err)
	}
	if len(enterpriseState().RestartRequired) != 1 {
		t.Fatal("native launch-time metadata was reported as hot-applied")
	}
	if err := settings.SetEnterprise(nil, enterpriseState().Revision); err != nil {
		t.Fatal(err)
	}
	if len(enterpriseState().RestartRequired) != 0 {
		t.Fatal("returning to the launch brand leaves a stale restart reminder")
	}
}
