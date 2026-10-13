package gui

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/yetone/magpie/internal/plugin"
	"github.com/yetone/magpie/internal/provider"
)

func pluginUpload(rw http.ResponseWriter, r *http.Request, w Windows) {
	// A cross-origin HTML form must not silently install executable code.
	// This header makes browser uploads require a same-origin fetch/preflight.
	if r.Header.Get("X-Magpie-Upload") != "zip" {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusForbidden)
		writeJSON(rw, map[string]string{"error": "plugin ZIP must be uploaded from the Plugins page"})
		return
	}
	r.Body = http.MaxBytesReader(rw, r.Body, plugin.MaxZIPSize+(64<<10))
	defer r.Body.Close()
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		fail(rw, err)
		return
	}
	defer r.MultipartForm.RemoveAll()
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["archive"]) != 1 {
		fail(rw, errors.New("choose one plugin ZIP to upload"))
		return
	}
	f, header, err := r.FormFile("archive")
	if err != nil {
		fail(rw, err)
		return
	}
	defer f.Close()
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
		fail(rw, errors.New("choose a .zip plugin archive"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if _, err := plugin.AddZIP(ctx, f, header.Size); err != nil {
		fail(rw, err)
		return
	}
	provider.HandOver(ctx, false)
	writeJSON(rw, pluginsState(ctx, w))
}
