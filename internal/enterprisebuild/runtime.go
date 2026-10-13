package enterprisebuild

import (
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yetone/magpie/internal/distribution"
	"github.com/yetone/magpie/internal/proc"
)

type Runtime struct {
	SchemaVersion int    `json:"schemaVersion"`
	Format        string `json:"format"`
	SourceCommit  string `json:"sourceCommit"`
	Dirty         bool   `json:"dirty"`
	Version       string `json:"version"`
	Target        string `json:"target"`
	SHA256        string `json:"sha256"`
	PlistSHA256   string `json:"plistSha256,omitempty"`
}

func emptyOutput(path string) error {
	if path == "" {
		return fmt.Errorf("output directory is required")
	}
	if entries, err := os.ReadDir(path); err == nil {
		if len(entries) != 0 {
			return fmt.Errorf("output directory is not empty")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.MkdirAll(path, 0o700)
}

func validTarget(target string) bool {
	return target == "darwin/arm64" || target == "darwin/amd64" || target == "windows/arm64" || target == "windows/amd64"
}

func fileHash(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// BuildRuntime compiles once per source/OS/architecture. Subsequent Build calls
// with BaseRuntime package configuration and resources without a Go compiler.
func BuildRuntime(ctx context.Context, o Options) (Runtime, error) {
	var r Runtime
	if !validTarget(o.Target) {
		return r, fmt.Errorf("unsupported runtime target")
	}
	if err := distribution.ValidateVersion(o.Version, 1); err != nil {
		return r, err
	}
	mac := strings.HasPrefix(o.Target, "darwin/")
	if mac && runtime.GOOS != "darwin" {
		return r, fmt.Errorf("macOS runtime needs a macOS worker")
	}
	work, err := os.MkdirTemp("", "enterprise-runtime-source-")
	if err != nil {
		return r, err
	}
	defer os.RemoveAll(work)
	commit, dirty, err := snapshot(ctx, o.Source, work, o.AllowDirty)
	if err != nil {
		return r, err
	}
	if o.SourceCommit != "" && o.SourceCommit != commit {
		return r, fmt.Errorf("approved source changed")
	}
	if err := os.WriteFile(filepath.Join(work, "internal", "distribution", "profile.json"), []byte("{}\n"), 0o600); err != nil {
		return r, err
	}
	if err := emptyOutput(o.Output); err != nil {
		return r, err
	}
	if !mac {
		if err := run(ctx, work, "go", "run", "github.com/tc-hib/go-winres@v0.3.3", "make", "--in", filepath.Join(work, "build", "windows", "winres.json"), "--arch", strings.Split(o.Target, "/")[1], "--out", "rsrc"); err != nil {
			return r, err
		}
	}
	flags := "-s -w -X main.version=" + o.Version + " -X github.com/yetone/magpie/internal/distribution.resourcePackage=1"
	if !mac {
		flags += " -H windowsgui"
	}
	cmd := proc.CommandContext(ctx, "go", "build", "-tags", "production", "-trimpath", "-ldflags", flags, "-o", filepath.Join(o.Output, "runtime"), ".")
	cmd.Dir = work
	cmd.Env = append(os.Environ(), "GOOS="+strings.Split(o.Target, "/")[0], "GOARCH="+strings.Split(o.Target, "/")[1], "CGO_ENABLED=0")
	if mac {
		cmd.Env = append(cmd.Env, "CGO_ENABLED=1", "CGO_CFLAGS=-mmacosx-version-min=12.0", "CGO_LDFLAGS=-mmacosx-version-min=12.0")
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return r, err
	}
	r = Runtime{SchemaVersion: 1, Format: "package-resources-v1", SourceCommit: commit, Dirty: dirty, Version: o.Version, Target: o.Target}
	r.SHA256, err = fileHash(filepath.Join(o.Output, "runtime"))
	if err != nil {
		return r, err
	}
	if mac {
		if err := copyFile(filepath.Join(work, "build", "darwin", "Info.plist"), filepath.Join(o.Output, "Info.plist"), 0o644); err != nil {
			return r, err
		}
		r.PlistSHA256, err = fileHash(filepath.Join(o.Output, "Info.plist"))
		if err != nil {
			return r, err
		}
	}
	b, _ := json.MarshalIndent(r, "", "  ")
	return r, os.WriteFile(filepath.Join(o.Output, "runtime-metadata.json"), append(b, '\n'), 0o600)
}

func ReadRuntime(dir string) (Runtime, error) {
	var r Runtime
	b, err := os.ReadFile(filepath.Join(dir, "runtime-metadata.json"))
	if err != nil || len(b) > 128<<10 {
		return r, fmt.Errorf("cannot read runtime metadata")
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return r, err
	}
	if r.SchemaVersion != 1 || r.Format != "package-resources-v1" || !validTarget(r.Target) || len(r.SourceCommit) != 40 {
		return r, fmt.Errorf("invalid runtime metadata")
	}
	if err := distribution.ValidateVersion(r.Version, 1); err != nil {
		return r, err
	}
	for _, item := range []struct{ name, hash string }{{"runtime", r.SHA256}, {"Info.plist", r.PlistSHA256}} {
		if item.name == "Info.plist" && strings.HasPrefix(r.Target, "windows/") {
			continue
		}
		fi, err := os.Lstat(filepath.Join(dir, item.name))
		if err != nil || !fi.Mode().IsRegular() {
			return r, fmt.Errorf("runtime file must be regular: %s", item.name)
		}
		got, err := fileHash(filepath.Join(dir, item.name))
		if err != nil || got != item.hash {
			return r, fmt.Errorf("runtime hash mismatch: %s", item.name)
		}
	}
	info, err := buildinfo.ReadFile(filepath.Join(dir, "runtime"))
	if err != nil {
		return r, err
	}
	settings := map[string]string{}
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	if settings["GOOS"]+"/"+settings["GOARCH"] != r.Target {
		return r, fmt.Errorf("binary is not a reusable enterprise runtime for %s", r.Target)
	}
	return r, nil
}
