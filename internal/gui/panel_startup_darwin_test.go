//go:build darwin && cgo && !nogui

package gui

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// The real WKWebView uses native panel options and the shipped page, but only
// synthetic API replies. runAppKit isolates AppKit/WebKit's account folders.
func TestNativeEnterprisePanelStartupAndAllowances(t *testing.T) {
	out, err := runAppKit(t, 30*time.Second, []string{"MAGPIE_TEST_PANEL_STARTUP=1"})
	if err != nil || !strings.Contains(string(out), "native enterprise panel: agents and allowances stay open") {
		t.Fatalf("native enterprise panel: %v\n%s", err, out)
	}
}

func init() {
	if os.Getenv("MAGPIE_TEST_PANEL_STARTUP") != "1" {
		return
	}
	files, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err)
	}
	static := http.FileServer(http.FS(files))
	result := make(chan string, 8)
	var panel atomic.Pointer[application.WebviewWindow]
	effective := map[string]any{"schemaVersion": 1, "appName": "msai-client", "visibleAgents": []string{"workbuddy"}, "visibleProviders": []string{}, "libraryEnabled": false, "publicPluginCatalog": false}
	enterprise := map[string]any{"effective": effective, "defaults": effective, "revision": "one", "restartRequired": []string{}, "updatePolicy": map[string]any{"mode": "manual"}}
	prefs := map[string]any{"lang": "zh", "theme": "dark", "textSize": 125, "guideHidden": true}
	boot := map[string]any{"lang": "zh", "theme": "dark", "textSize": 125, "web": false, "enterprise": enterprise}
	agent := map[string]any{"id": "workbuddy", "name": "WorkBuddy", "icon": "generic", "path": "/fixture/workbuddy", "wired": true, "fields": []any{map[string]any{"key": "model", "label": "model", "value": "corp/test-model", "options": []any{map[string]string{"value": "corp/test-model", "label": "Test Model"}}}}}
	const probe = `<script>
window.addEventListener('error', e => fetch('/native/result?error=' + encodeURIComponent(e.message)));
window.addEventListener('unhandledrejection', e => fetch('/native/result?error=' + encodeURIComponent(String(e.reason))));
let nativeStage = 0;
function nativePickAllowances() {
  nativeStage = 2;
  const tab = document.querySelector('[data-ptab="usage"]'); tab.focus({preventScroll:true}); tab.click();
}
const nativeProbe = setInterval(() => {
  const row = document.querySelector('.row.agent[data-id="workbuddy"]');
  if (nativeStage === 0 && row && row.getBoundingClientRect().height > 0 && row.textContent.includes('Test Model')) {
    nativeStage = 1; fetch('/native/result?stage=ready');
  } else if (nativeStage === 2 && document.body.dataset.ptab === 'usage' && document.querySelector('#panelQuota .pq-card')) {
    nativeStage = 3; fetch('/native/result?stage=allowances');
    const agents = document.querySelector('[data-ptab="agents"]'); agents.focus({preventScroll:true}); agents.click();
  } else if (nativeStage === 3 && document.body.dataset.ptab === 'agents' && row?.getBoundingClientRect().height > 0) {
    clearInterval(nativeProbe); fetch('/native/result?stage=agents');
  }
}, 30);
</script>`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		write := func(v any) { w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(v) }
		switch r.URL.Path {
		case "/":
			b, err := fs.ReadFile(files, "index.html")
			if err != nil {
				panic(err)
			}
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, strings.Replace(string(b), "</head>", probe+"</head>", 1))
		case "/boot.js":
			b, _ := json.Marshal(boot)
			w.Header().Set("Content-Type", "text/javascript")
			fmt.Fprintf(w, "window.bootPrefs=%s;", b)
		case "/api/state":
			write(map[string]any{"agents": []any{agent}, "profiles": []any{}, "served": 1, "settings": prefs})
		case "/api/enterprise":
			write(enterprise)
		case "/api/usage/quotas":
			write([]any{map[string]any{"provider": "copilot", "name": "Copilot", "icon": "githubcopilot", "user": "fixture", "windows": []any{map[string]any{"name": "Premium requests", "used": 8}}}})
		case "/api/providers":
			write(map[string]any{"providers": []any{}, "presets": []any{}, "excluded": []any{}, "plugins": []any{}, "gateway": map[string]any{"running": true, "url": "http://127.0.0.1:3999", "groups": []any{}, "calls": []any{}}})
		case "/api/groups":
			write(map[string]any{"groups": []any{}, "pools": []any{}})
		case "/api/usage":
			write(map[string]any{"calls": 0, "agents": []any{}, "models": []any{}, "series": []any{}})
		case "/api/window/fit":
			if h, _, ok := parseFit(r.URL.Query()); ok {
				application.InvokeSync(func() {
					if p := panel.Load(); p != nil {
						p.SetSize(zoomed(panelWidth, 1.25), min(700, zoomed(h, 1.25)))
					}
				})
			}
			w.WriteHeader(http.StatusNoContent)
		case "/native/result":
			if e := r.URL.Query().Get("error"); e != "" {
				result <- "error: " + e
			} else {
				result <- r.URL.Query().Get("stage")
			}
			w.WriteHeader(http.StatusNoContent)
		case "/api/window/hide", "/api/window/main", "/api/window/quit":
			result <- "error: a panel tab requested " + r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		default:
			if strings.HasPrefix(r.URL.Path, "/api/") {
				write(map[string]any{})
			} else {
				static.ServeHTTP(w, r)
			}
		}
	})
	app := application.New(application.Options{
		Name:   "enterprise panel regression",
		Mac:    application.MacOptions{ActivationPolicy: application.ActivationPolicyAccessory},
		Assets: application.AssetOptions{Handler: handler},
	})
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		application.InvokeAsync(func() {
			o := panelOptions("darwin", "")
			o.Width = zoomed(panelWidth, 1.25)
			p := app.Window.NewWithOptions(o)
			panel.Store(p)
			setPageZoom(p, 1.25)
			p.Show().Focus()
		})
	})
	go func() {
		if got := <-result; got != "ready" {
			fmt.Println("native enterprise panel:", got)
			os.Exit(1)
		}
		// An API reply may arrive before native Show/Focus has finished. Begin
		// the tab gesture only after AppKit says the panel is the key window.
		application.InvokeSync(func() { panel.Load().Show().Focus() })
		deadline := time.Now().Add(5 * time.Second)
		for !application.InvokeSyncWithResult(func() bool { return panel.Load().IsFocused() }) {
			if time.Now().After(deadline) {
				fmt.Println("native enterprise panel: initial focus did not settle")
				os.Exit(1)
			}
			time.Sleep(10 * time.Millisecond)
		}
		application.InvokeSync(func() { panel.Load().ExecJS("nativePickAllowances();") })
		for _, want := range []string{"allowances", "agents"} {
			if got := <-result; got != want {
				fmt.Println("native enterprise panel:", got)
				os.Exit(1)
			}
			if !application.InvokeSyncWithResult(func() bool { return windowOpen(panel.Load()) }) {
				fmt.Println("native enterprise panel: the window hid when switching", want)
				os.Exit(1)
			}
		}
		fmt.Println("native enterprise panel: agents and allowances stay open")
		app.Quit()
	}()
	if err := app.Run(); err != nil {
		panic(err)
	}
	os.Exit(0)
}
