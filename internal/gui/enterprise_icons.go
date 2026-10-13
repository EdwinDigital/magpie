package gui

import (
	"fmt"
	"os"
	"sync"

	"github.com/yetone/magpie/internal/brandicon"
	"github.com/yetone/magpie/internal/settings"
)

var enterpriseIconCache = struct {
	sync.Mutex
	items map[string]*brandicon.Icons
	order []string
}{items: map[string]*brandicon.Icons{}}

func enterpriseIcons(asset string) (*brandicon.Icons, error) {
	b, err := enterpriseLogo(asset) // Verify the source even on a cache hit.
	if err != nil || len(b) == 0 {
		return nil, fmt.Errorf("application logo is unavailable")
	}
	enterpriseIconCache.Lock()
	defer enterpriseIconCache.Unlock()
	if icons := enterpriseIconCache.items[asset]; icons != nil {
		return icons, nil
	}
	icons, err := brandicon.Generate(b)
	if err != nil {
		return nil, err
	}
	if len(enterpriseIconCache.order) == 8 {
		delete(enterpriseIconCache.items, enterpriseIconCache.order[0])
		enterpriseIconCache.order = enterpriseIconCache.order[1:]
	}
	enterpriseIconCache.items[asset] = icons
	enterpriseIconCache.order = append(enterpriseIconCache.order, asset)
	return icons, nil
}

func enterpriseIconURLs(asset string) map[string]string {
	base := "/api/enterprise/logo/" + asset[len("sha256:"):] + ".png?v=" + brandicon.Version
	return map[string]string{"light": base + "&theme=light", "dark": base + "&theme=dark"}
}

func enterpriseThemeLogo(asset, theme string, app bool) []byte {
	icons, err := enterpriseIcons(asset)
	if err != nil {
		return nil
	}
	if app {
		if theme == "dark" {
			return icons.AppDark
		}
		return icons.AppLight
	}
	if theme == "dark" {
		return icons.Dark
	}
	return icons.Light
}

var enterpriseSystemDark = func() bool { return false }

func enterpriseIconTheme() string {
	if theme := os.Getenv("MAGPIE_THEME"); theme == "light" || theme == "dark" {
		return theme
	}
	theme := settings.Load().Theme
	if theme == "dark" || theme == "light" {
		return theme
	}
	if enterpriseSystemDark() {
		return "dark"
	}
	return "light"
}

func brandAppIcon() []byte {
	return enterpriseThemeLogo(settings.ResolveEnterprise(settings.Load()).LogoAsset, enterpriseIconTheme(), true)
}
