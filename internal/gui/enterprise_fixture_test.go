package gui

import (
	"net/http"
	"testing"

	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/settings"
)

// Legacy catalog tests explicitly open the catalog. The enterprise tests
// exercise the real closed defaults instead of weakening production guards.
func enterpriseCatalogFixture(t *testing.T, w Windows, gw *gateway.Server) http.Handler {
	t.Helper()
	e := settings.ResolveEnterprise(settings.Load())
	e.PublicPluginCatalog = true
	e.LibraryEnabled = true
	e.OverrideFields = append(e.OverrideFields, "publicPluginCatalog", "libraryEnabled")
	if err := settings.SetEnterprise(&e, settings.EnterpriseRevision(settings.Load())); err != nil {
		t.Fatal(err)
	}
	return Handler(w, gw)
}
