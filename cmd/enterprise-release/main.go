// enterprise-release builds branded packages without changing the source
// checkout and serves a tenant-scoped API for a future distribution portal.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/distribution"
	"github.com/yetone/magpie/internal/enterprisebuild"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: enterprise-release prepare|runtime|build|manifest|serve|enroll [options]")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	var o enterprisebuild.Options
	fs.StringVar(&o.ProfilePath, "profile", "", "release profile JSON")
	fs.StringVar(&o.LogoPath, "logo", "", "PNG/JPEG logo")
	fs.StringVar(&o.Output, "out", "", "new output directory")
	fs.StringVar(&o.BaseRuntime, "base-runtime", "", "approved precompiled runtime directory; no app compilation")
	fs.StringVar(&o.Source, "source", ".", "approved source checkout")
	fs.StringVar(&o.Target, "target", "", "darwin/arm64, darwin/amd64, windows/amd64 or windows/arm64")
	fs.StringVar(&o.Version, "version", "", "enterprise x.y.z release version")
	fs.IntVar(&o.BuildNumber, "build-number", 1, "platform build number")
	fs.BoolVar(&o.Unsigned, "unsigned", false, "development packages only; cannot publish")
	fs.BoolVar(&o.AllowDirty, "allow-dirty", false, "unsigned development snapshot only")
	fs.StringVar(&o.MacIdentity, "mac-identity", "", "worker's authorized Developer ID Application identity")
	fs.StringVar(&o.NotaryProfile, "notary-profile", "", "worker's notarytool keychain profile")
	fs.StringVar(&o.WindowsThumbprint, "windows-thumbprint", "", "worker's Authenticode certificate thumbprint")
	fs.StringVar(&o.TimestampURL, "timestamp-url", "", "trusted RFC3161 timestamp service")
	metadata := fs.String("metadata", "", "build-metadata.json")
	privateKey := fs.String("private-key", "", "worker's manifest signing key file")
	keyID := fs.String("key-id", "", "embedded trusted key ID")
	baseURL := fs.String("base-url", "", "HTTPS artifact directory URL")
	sequence := fs.Uint64("sequence", 0, "monotonic publication sequence")
	listen := fs.String("listen", "127.0.0.1:3460", "distribution API address")
	access := fs.String("access-config", "", "private tenant access configuration")
	distributionID := fs.String("distribution-id", "", "employee package identity for token enrollment")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if o.Output != "" {
		var err error
		o.Output, err = filepath.Abs(o.Output)
		if err != nil {
			return err
		}
	}
	switch args[0] {
	case "enroll":
		if !regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`).MatchString(*distributionID) {
			return fmt.Errorf("a valid distribution identity is required")
		}
		if distribution.ID() != "" && distribution.ID() != *distributionID {
			return fmt.Errorf("the credential belongs to another distribution")
		}
		b, err := io.ReadAll(io.LimitReader(os.Stdin, 8193))
		if err != nil {
			return err
		}
		token := strings.TrimSpace(string(b))
		if len(token) < 32 || len(token) > 8192 || strings.ContainsAny(token, "\r\n\t ") {
			return fmt.Errorf("invalid enrollment token")
		}
		dir := appdir.Config()
		if distribution.ID() == "" {
			p := distribution.Profile{DistributionID: *distributionID}
			p.Identity.ArtifactSlug = *distributionID
			if o.ProfilePath != "" {
				b, err := os.ReadFile(o.ProfilePath)
				if err != nil {
					return err
				}
				if len(b) > 128<<10 {
					return fmt.Errorf("profile is too large")
				}
				if err := json.Unmarshal(b, &p); err != nil {
					return err
				}
				if err := distribution.Validate(p); err != nil {
					return err
				}
				if p.DistributionID != *distributionID {
					return fmt.Errorf("profile belongs to another distribution")
				}
			}
			dir = appdir.ConfigForDistribution(p)
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		path := filepath.Join(dir, "enterprise-update-token")
		if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
			return err
		}
		return os.Chmod(path, 0o600)
	case "prepare":
		if o.ProfilePath == "" || o.LogoPath == "" || o.Output == "" {
			return fmt.Errorf("prepare requires --profile, --logo and --out")
		}
		p, err := enterprisebuild.Prepare(o.ProfilePath, o.LogoPath, o.Output)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(p)
	case "runtime":
		if o.Output == "" {
			return fmt.Errorf("runtime requires --out")
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		r, err := enterprisebuild.BuildRuntime(ctx, o)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(r)
	case "build":
		if o.ProfilePath == "" || o.LogoPath == "" || o.Output == "" {
			return fmt.Errorf("build requires --profile, --logo and --out")
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		result, err := enterprisebuild.Build(ctx, o)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	case "manifest":
		result, err := enterprisebuild.ReadResult(*metadata)
		if err != nil {
			return err
		}
		key, err := enterprisebuild.ReadPrivateKey(*privateKey)
		if err != nil {
			return err
		}
		m, err := enterprisebuild.SignManifest(result, filepath.Dir(*metadata), *baseURL, *keyID, *sequence, key)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(m)
	case "serve":
		return serve(ctx, *listen, *access, o, *privateKey, *keyID, *baseURL)
	default:
		return fmt.Errorf("unknown enterprise-release operation %q", args[0])
	}
}
