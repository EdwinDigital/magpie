package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/update"
)

func TestEnterpriseManualUpdateIsAChannelStateNotAnError(t *testing.T) {
	oldPolicy, oldUpdater := packageUpdatePolicy, updates
	packageUpdatePolicy = func() update.Policy {
		return update.Policy{DistributionID: "acme-assistant", Mode: "manual", Channel: "stable"}
	}
	updates = &updater{state: "error", err: "this enterprise package uses manual updates"}
	t.Cleanup(func() { packageUpdatePolicy, updates = oldPolicy, oldUpdater })
	if updateDue(settings.Settings{UpdateEvery: 30}, time.Time{}, time.Now()) || updates.begin() {
		t.Fatal("manual enterprise package started automatic version detection")
	}
	mux := http.NewServeMux()
	updateRoutes(mux, nil)
	for _, tc := range []struct{ method, path string }{{"GET", "/api/update"}, {"POST", "/api/update/check"}, {"POST", "/api/update/install"}} {
		rw := httptest.NewRecorder()
		mux.ServeHTTP(rw, httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}")))
		var j updateJSON
		if rw.Code != 200 || json.Unmarshal(rw.Body.Bytes(), &j) != nil || j.State != "manual" || j.Error != "" || j.Policy == nil || j.Policy.DistributionID != "acme-assistant" {
			t.Fatalf("%s: %d %s", tc.path, rw.Code, rw.Body)
		}
	}
}
