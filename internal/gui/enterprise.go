package gui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/yetone/magpie/internal/agent"
	"github.com/yetone/magpie/internal/brandicon"
	"github.com/yetone/magpie/internal/distribution"
	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/plugin"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/update"
)

var onEnterprise func()
var enterprisePendingRestart func(settings.Enterprise) []string
var configurationRestart atomic.Bool

type enterpriseJSON struct {
	Config          *settings.Enterprise `json:"config"`
	Effective       settings.Enterprise  `json:"effective"`
	Defaults        settings.Enterprise  `json:"defaults"`
	Revision        string               `json:"revision"`
	LogoURL         string               `json:"logoUrl"`
	LogoURLs        map[string]string    `json:"logoUrls,omitempty"`
	DistributionID  string               `json:"distributionId"`
	ProfileRevision int                  `json:"profileRevision"`
	RestartRequired []string             `json:"restartRequired"`
	UpdatePolicy    *update.Policy       `json:"updatePolicy,omitempty"`
}

func enterpriseState() enterpriseJSON {
	s := settings.Load()
	e := settings.ResolveEnterprise(s)
	u := ""
	if e.LogoAsset != "" {
		u = enterpriseIconURLs(e.LogoAsset)["light"]
	}
	restart := []string{}
	if enterprisePendingRestart != nil {
		restart = enterprisePendingRestart(e)
	}
	state := enterpriseJSON{Config: s.Enterprise, Effective: e, Defaults: settings.EnterpriseDefaults(), Revision: settings.EnterpriseRevision(s), LogoURL: u, DistributionID: distribution.ID(), ProfileRevision: distribution.Current().ProfileRevision, RestartRequired: restart}
	if e.LogoAsset != "" {
		state.LogoURLs = enterpriseIconURLs(e.LogoAsset)
	}
	if distribution.ID() != "" {
		policy := update.CurrentPolicy()
		state.UpdatePolicy = &policy
	}
	return state
}

func brandName() string { return settings.ResolveEnterprise(settings.Load()).AppName }

func enterpriseHTML(page []byte) []byte {
	s := enterpriseState()
	name := html.EscapeString(s.Effective.AppName)
	page = bytes.Replace(page, []byte("<title>magpie</title>"), []byte("<title>"+name+"</title>"), 1)
	page = bytes.Replace(page, []byte("<span>magpie</span>"), []byte("<span>"+name+"</span>"), 1)
	if !s.Effective.LibraryEnabled {
		page = bytes.Replace(page, []byte(`<button data-view="library"`), []byte(`<button hidden data-view="library"`), 1)
	}
	if s.LogoURL != "" {
		images := `<img class="enterprise-logo-light" src="` + html.EscapeString(s.LogoURLs["light"]) + `" width="18" height="18" alt=""><img class="enterprise-logo-dark" src="` + html.EscapeString(s.LogoURLs["dark"]) + `" width="18" height="18" alt="">`
		page = regexp.MustCompile(`<span class="logo" aria-hidden="true">.*?</span>`).ReplaceAll(page, []byte(`<span class="logo" aria-hidden="true">`+images+`</span>`))
		page = bytes.Replace(page, []byte(`href="icons/favicon.svg" type="image/svg+xml"`), []byte(`href="`+s.LogoURL+`" type="image/png"`), 1)
	}
	return page
}

func enterpriseLogo(asset string) ([]byte, error) {
	if asset == "" {
		return nil, nil
	}
	if err := settings.ValidateEnterprise(&settings.Enterprise{SchemaVersion: 1, LogoAsset: asset}); err != nil {
		return nil, err
	}
	if asset == distribution.Current().Brand.LogoAsset && distribution.ID() != "" {
		return distribution.Logo(), nil
	}
	b, err := os.ReadFile(filepath.Join(settings.Dir(), "branding", strings.TrimPrefix(asset, "sha256:")+".png"))
	if err != nil {
		return nil, err
	}
	if distribution.AssetID(b) != asset {
		return nil, fmt.Errorf("logo integrity check failed")
	}
	return b, nil
}

func brandLogo() []byte {
	return enterpriseThemeLogo(settings.ResolveEnterprise(settings.Load()).LogoAsset, enterpriseIconTheme(), false)
}

type enterpriseProvider struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Default   bool   `json:"default"`
	Available bool   `json:"available"`
	Icon      string `json:"icon,omitempty"`
}

func enterpriseProviders() []enterpriseProvider {
	out := []enterpriseProvider{}
	have := map[string]bool{}
	add := func(id, name string, icons ...string) {
		if have[id] {
			return
		}
		have[id] = true
		icon := ""
		if len(icons) > 0 {
			icon = icons[0]
		}
		out = append(out, enterpriseProvider{id, name, id == "subscription:copilot" || id == "preset:ollama", true, icon})
	}
	for _, s := range []struct{ id, name string }{
		{"claude", "Claude"}, {"codex", "ChatGPT"}, {"chatgpt-api", "ChatGPT API"}, {"cursor", "Cursor"}, {"grok", "Grok"}, {"copilot", "Copilot"}, {"zcode", "ZCode"}, {"workbuddy", "WorkBuddy"}, {"workbuddy-ai", "WorkBuddy AI"}, {"commandcode-plan", "Command Code"}, {"qoder", "Qoder"}, {"qoder-cn", "Qoder CN"}, {"devin", "Devin"}, {"zed", "Zed"}, {"factory", "Factory"}, {"mimo-app", "Xiaomi MiMo"}, {"kiro", "Kiro"}, {"gemini", "Gemini"}, {"antigravity", "Antigravity"},
	} {
		add("subscription:"+s.id, s.name)
	}
	for _, p := range provider.Presets() {
		add("preset:"+p.ID, p.Name, p.Icon)
	}
	for _, p := range provider.Partners() {
		add("preset:"+p.ID, p.Name)
	}
	for _, p := range provider.All() {
		id := "custom:" + p.ID
		if p.Account != nil {
			id = "subscription:" + p.Account.Agent
			if p.IsPlugin() {
				id = "plugin:" + p.ID
				if pp, ok := provider.PluginOf(p.ID); ok && provider.Moved(pp.ID) {
					id = "subscription:" + pp.ID
				}
			}
		} else if p.Preset != "" {
			id = "preset:" + p.Preset
		}
		add(id, p.Name, p.Icon)
	}
	for _, p := range pluginSubs() {
		id := "plugin:" + p.ID
		if slices.Contains(provider.OnPlugins(), p.PID) {
			id = "subscription:" + p.PID
		}
		add(id, p.Name)
	}
	for _, id := range settings.ResolveEnterprise(settings.Load()).VisibleProviders {
		if !have[id] {
			out = append(out, enterpriseProvider{id, id, false, false, ""})
		}
	}
	slices.SortFunc(out, func(a, b enterpriseProvider) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func enterpriseRoutes(mux *http.ServeMux, w Windows) {
	mux.HandleFunc("GET /api/enterprise/agents", func(rw http.ResponseWriter, r *http.Request) {
		out := []map[string]any{}
		have := map[string]bool{}
		for _, a := range agent.All() {
			if strings.Contains(a.ID, "@wsl:") || strings.Contains(a.ID, "#") {
				continue
			}
			if !have[a.ID] {
				out = append(out, map[string]any{"id": a.ID, "name": a.Name, "icon": a.Icon, "available": true})
				have[a.ID] = true
			}
		}
		for _, id := range settings.ResolveEnterprise(settings.Load()).VisibleAgents {
			if !have[id] {
				out = append(out, map[string]any{"id": id, "name": id, "available": false})
			}
		}
		writeJSON(rw, out)
	})
	mux.HandleFunc("GET /api/enterprise", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Cache-Control", "no-store")
		writeJSON(rw, enterpriseState())
	})
	mux.HandleFunc("GET /api/enterprise/providers", func(rw http.ResponseWriter, r *http.Request) { writeJSON(rw, enterpriseProviders()) })
	mux.HandleFunc("POST /api/enterprise", func(rw http.ResponseWriter, r *http.Request) {
		var in struct {
			Config   *settings.Enterprise `json:"config"`
			Revision string               `json:"revision"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(rw, r.Body, 128<<10))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			fail(rw, err)
			return
		}
		if dec.Decode(new(any)) != io.EOF {
			fail(rw, fmt.Errorf("one JSON value is required"))
			return
		}
		if err := settings.ValidateEnterprise(in.Config); err != nil {
			fail(rw, err)
			return
		}
		if in.Config != nil && in.Config.LogoAsset != "" {
			if _, err := enterpriseLogo(in.Config.LogoAsset); err != nil {
				fail(rw, fmt.Errorf("logo is unavailable; choose the image again"))
				return
			}
		}
		before := settings.Load()
		if err := settings.SetEnterprise(in.Config, in.Revision); err != nil {
			if errors.Is(err, settings.ErrEnterpriseConflict) {
				rw.Header().Set("Content-Type", "application/json")
				rw.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(rw).Encode(map[string]string{"error": err.Error(), "code": "config_conflict"})
				return
			}
			fail(rw, err)
			return
		}
		old, next := settings.ResolveEnterprise(before), settings.ResolveEnterprise(settings.Load())
		if !slices.Equal(old.VisibleAgents, next.VisibleAgents) || !slices.Equal(old.VisibleProviders, next.VisibleProviders) || distribution.ID() == "" && (before.Enterprise == nil) != (in.Config == nil) {
			// Forget probe caches, never credentials, when the scan policy changes.
			agent.Rescan()
			provider.ForgetAccounts()
		}
		if onEnterprise != nil {
			onEnterprise()
		}
		writeJSON(rw, enterpriseState())
	})
	mux.HandleFunc("POST /api/enterprise/logo", func(rw http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(rw, r.Body, (2<<20)+(64<<10))
		if err := r.ParseMultipartForm(2 << 20); err != nil {
			fail(rw, err)
			return
		}
		defer r.MultipartForm.RemoveAll()
		f, _, err := r.FormFile("logo")
		if err != nil {
			fail(rw, err)
			return
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
		if err != nil || len(b) > 2<<20 {
			fail(rw, fmt.Errorf("logo must be at most 2 MiB"))
			return
		}
		icons, err := brandicon.Generate(b)
		if err != nil {
			fail(rw, err)
			return
		}
		id := distribution.AssetID(icons.Source)
		dir := filepath.Join(settings.Dir(), "branding")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			fail(rw, err)
			return
		}
		if err := edit.WriteAtomic(filepath.Join(dir, strings.TrimPrefix(id, "sha256:")+".png"), icons.Source); err != nil {
			fail(rw, err)
			return
		}
		writeJSON(rw, map[string]any{"asset": id, "url": enterpriseIconURLs(id)["light"], "logoUrls": enterpriseIconURLs(id)})
	})
	mux.HandleFunc("GET /api/enterprise/logo/{name}", func(rw http.ResponseWriter, r *http.Request) {
		id := "sha256:" + strings.TrimSuffix(r.PathValue("name"), ".png")
		b, err := enterpriseLogo(id)
		if err != nil {
			http.NotFound(rw, r)
			return
		}
		if theme := r.URL.Query().Get("theme"); theme != "" {
			if theme != "light" && theme != "dark" {
				http.NotFound(rw, r)
				return
			}
			b = enterpriseThemeLogo(id, theme, false)
			if len(b) == 0 {
				http.NotFound(rw, r)
				return
			}
		}
		rw.Header().Set("Content-Type", "image/png")
		rw.Header().Set("X-Content-Type-Options", "nosniff")
		rw.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		_, _ = rw.Write(b)
	})
	mux.HandleFunc("POST /api/application/restart", func(rw http.ResponseWriter, r *http.Request) {
		if w == nil || isWeb(w) {
			http.Error(rw, "restart is available only in the desktop app", http.StatusForbidden)
			return
		}
		if gatewayBusy().Any() {
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(rw).Encode(map[string]string{"error": "wait for running gateway requests before restarting"})
			return
		}
		if restartApplication == nil {
			http.Error(rw, "restart unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := restartApplication(); err != nil {
			fail(rw, err)
			return
		}
		writeJSON(rw, map[string]bool{"restarting": true})
	})
}

var restartApplication func() error

// Guard before the held catalog and before route-specific scans or network IO.
func enterpriseGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		e := settings.ResolveEnterprise(settings.Load())
		path := r.URL.Path
		blocked := (path == "/api/library" || strings.HasPrefix(path, "/api/library/")) && !e.LibraryEnabled
		if !e.PublicPluginCatalog {
			switch path {
			case "/api/plugins/listings", "/api/plugins/github", "/api/plugins/market", "/api/plugins/search":
				blocked = true
			}
			if path == "/api/plugins/npm" || path == "/api/plugins/page" {
				names := strings.Split(r.URL.Query().Get("names"), ",")
				if path == "/api/plugins/page" {
					names = []string{r.URL.Query().Get("name")}
				}
				installed := npmNames(plugin.Load().Plugins)
				for _, name := range names {
					if name != "" && !slices.Contains(installed, name) {
						blocked = true
					}
				}
			}
		}
		if blocked {
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(rw).Encode(map[string]string{"error": "this feature is disabled in enterprise settings", "code": "feature_disabled"})
			return
		}
		next.ServeHTTP(rw, r)
	})
}
