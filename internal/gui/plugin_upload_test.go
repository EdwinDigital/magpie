package gui

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/plugin"
	"github.com/yetone/magpie/internal/testenv"
)

func pluginZIPBody(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, f := range [][2]string{{"example/package.json", `{"name":"company-example","main":"index.js"}`}, {"example/index.js", `export default async () => ({})`}} {
		w, err := z.Create(f[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(f[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func uploadPluginRequest(t *testing.T, h http.Handler, name string, data []byte, header bool, copies int) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	m := multipart.NewWriter(&b)
	for i := 0; i < copies; i++ {
		w, err := m.CreateFormFile("archive", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/plugins/upload", &b)
	r.Header.Set("Content-Type", m.FormDataContentType())
	if header {
		r.Header.Set("X-Magpie-Upload", "zip")
	}
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, r)
	return rw
}

func TestPluginZIPUploadRoute(t *testing.T) {
	bun := testenv.Bun(t)
	sandboxHome(t)
	t.Setenv("MAGPIE_BUN", bun)
	t.Cleanup(plugin.Settle)
	// Multipart spills, when needed, stay in this test's private temporary dir.
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	mux := Handler(nil, nil)
	if r := enterpriseRequest(t, mux, "GET", "/api/plugins/listings", nil); r.Code != http.StatusForbidden {
		t.Fatalf("test did not close public discovery: %d %s", r.Code, r.Body)
	}
	b := pluginZIPBody(t)
	for _, tc := range []struct {
		name   string
		data   []byte
		header bool
		copies int
		status int
	}{
		{"plugin.zip", b, false, 1, http.StatusForbidden},
		{"plugin.js", b, true, 1, http.StatusBadRequest},
		{"plugin.zip", []byte("not a ZIP"), true, 1, http.StatusBadRequest},
		{"plugin.zip", b, true, 0, http.StatusBadRequest},
		{"plugin.zip", b, true, 2, http.StatusBadRequest},
		{"plugin.zip", bytes.Repeat([]byte("x"), plugin.MaxZIPSize+1), true, 1, http.StatusBadRequest},
	} {
		r := uploadPluginRequest(t, mux, tc.name, tc.data, tc.header, tc.copies)
		if r.Code != tc.status || len(plugin.Load().Plugins) != 0 {
			t.Fatalf("%s: %d %s installed=%+v", tc.name, r.Code, r.Body, plugin.Load())
		}
	}
	r := uploadPluginRequest(t, mux, "company-plugin.ZIP", b, true, 1)
	var state pluginsJSON
	if r.Code != http.StatusOK || json.Unmarshal(r.Body.Bytes(), &state) != nil || len(state.Plugins) != 1 {
		t.Fatalf("upload did not return installed plugin state: %d %s", r.Code, r.Body)
	}
	e := state.Plugins[0]
	if _, err := os.Stat(filepath.Join(e.Spec, "index.js")); err != nil || e.Archive == "" {
		t.Fatalf("upload files not retained: %+v %v", e, err)
	}
	es, err := os.ReadDir(tmp)
	if err != nil || len(es) != 0 {
		t.Fatalf("multipart temporary files leaked: %v %v", es, err)
	}
}
