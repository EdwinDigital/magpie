// Package enterprisebuild produces isolated, versioned enterprise artifacts.
// Signing identities are worker inputs, never part of the embedded profile.
package enterprisebuild

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/yetone/magpie/internal/brandicon"
	"github.com/yetone/magpie/internal/distribution"
	"github.com/yetone/magpie/internal/proc"
)

type Options struct {
	SourceCommit                                           string
	Progress                                               func(string)
	Source, Output, ProfilePath, LogoPath, Target, Version string
	BaseRuntime, BaseSHA256                                string
	BuildNumber                                            int
	Unsigned, AllowDirty                                   bool
	MacIdentity, NotaryProfile, WindowsThumbprint          string
	TimestampURL                                           string
}

type Result struct {
	Profile        distribution.Profile          `json:"profile"`
	SourceCommit   string                        `json:"sourceCommit"`
	Dirty          bool                          `json:"dirty"`
	Version        string                        `json:"version"`
	BuildNumber    int                           `json:"buildNumber"`
	Target         string                        `json:"target"`
	Signed         bool                          `json:"signed"`
	RuntimeVersion string                        `json:"runtimeVersion,omitempty"`
	RuntimeSHA256  string                        `json:"runtimeSha256,omitempty"`
	Assets         map[string]distribution.Asset `json:"assets"`
}

var version = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func SourceCommit(ctx context.Context, source string) (string, error) {
	cmd := proc.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = source
	b, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func readProfile(path string) (distribution.Profile, error) {
	var p distribution.Profile
	f, err := os.Open(path)
	if err != nil {
		return p, err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 128<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return p, err
	}
	if d.Decode(new(any)) != io.EOF {
		return p, fmt.Errorf("one JSON profile is required")
	}
	return p, nil
}

func normalizeLogo(path string) ([]byte, image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	if err != nil || len(b) > 2<<20 {
		return nil, nil, fmt.Errorf("logo exceeds 2 MiB")
	}
	icons, err := brandicon.Generate(b)
	if err != nil {
		return nil, nil, err
	}
	img, err := png.Decode(bytes.NewReader(icons.Source))
	return icons.Source, img, err
}

func scale(src image.Image, n int) *image.NRGBA {
	b := src.Bounds()
	width, height := n, n
	if b.Dx() > b.Dy() {
		height = n * b.Dy() / b.Dx()
	} else {
		width = n * b.Dx() / b.Dy()
	}
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	out := image.NewNRGBA(image.Rect(0, 0, n, n))
	ox, oy := (n-width)/2, (n-height)/2
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			out.SetNRGBA(ox+x, oy+y, color.NRGBAModel.Convert(src.At(b.Min.X+x*b.Dx()/width, b.Min.Y+y*b.Dy()/height)).(color.NRGBA))
		}
	}
	return out
}

// Prepare writes only to a new or empty directory, never to the source tree.
func Prepare(profilePath, logoPath, out string) (distribution.Profile, error) {
	p, err := readProfile(profilePath)
	if err != nil {
		return p, err
	}
	b, img, err := normalizeLogo(logoPath)
	if err != nil {
		return p, err
	}
	id := distribution.AssetID(b)
	if p.Brand.LogoAsset != "" && p.Brand.LogoAsset != id {
		// Older prepared profiles froze a different PNG normalization. Keep
		// its verified source identity, while deriving the new theme assets.
		original, err := os.ReadFile(logoPath)
		if err != nil || distribution.AssetID(original) != p.Brand.LogoAsset {
			return p, fmt.Errorf("logo does not match the frozen profile")
		}
		img, err = png.Decode(bytes.NewReader(original))
		if err != nil {
			return p, fmt.Errorf("logo does not match the frozen profile")
		}
		b, id = original, p.Brand.LogoAsset
	}
	p.Brand.LogoAsset = id
	if err := distribution.Validate(p); err != nil {
		return p, err
	}
	if entries, err := os.ReadDir(out); err == nil && len(entries) > 0 {
		return p, fmt.Errorf("output directory is not empty")
	} else if err != nil && !os.IsNotExist(err) {
		return p, err
	}
	if err := os.MkdirAll(out, 0o700); err != nil {
		return p, err
	}
	profile, _ := json.MarshalIndent(p, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "profile.json"), append(profile, '\n'), 0o600); err != nil {
		return p, err
	}
	if err := os.WriteFile(filepath.Join(out, "logo.png"), b, 0o600); err != nil {
		return p, err
	}
	icons, err := brandicon.Generate(b)
	if err != nil {
		return p, err
	}
	for name, data := range map[string][]byte{"logo-light.png": icons.Light, "logo-dark.png": icons.Dark, "app-light.png": icons.AppLight, "app-dark.png": icons.AppDark} {
		if err := os.WriteFile(filepath.Join(out, name), data, 0o600); err != nil {
			return p, err
		}
	}
	for _, variant := range []struct {
		file, prefix string
		data         []byte
	}{{"enterprise.icns", "icon-", icons.AppLight}, {"enterprise-dark.icns", "icon-dark-", icons.AppDark}} {
		img, err = png.Decode(bytes.NewReader(variant.data))
		if err != nil {
			return p, err
		}
		if err := writeICNS(out, variant.file, variant.prefix, img); err != nil {
			return p, err
		}
	}
	return p, nil
}

func writeICNS(out, name, prefix string, img image.Image) error {
	var icns bytes.Buffer
	icns.WriteString("icns")
	_ = binary.Write(&icns, binary.BigEndian, uint32(0))
	for _, item := range []struct {
		size int
		kind string
	}{{16, ""}, {32, "ic11"}, {64, "ic12"}, {128, "ic07"}, {256, "ic08"}, {512, "ic09"}, {1024, "ic10"}} {
		var asset bytes.Buffer
		if err := png.Encode(&asset, scale(img, item.size)); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, prefix+strconv.Itoa(item.size)+".png"), asset.Bytes(), 0o600); err != nil {
			return err
		}
		// PNG-backed Retina slots are supported by macOS. icp4/5/6 are
		// legacy pixel slots and can misdecode these small PNGs as RGB data.
		if item.kind == "" {
			continue
		}
		icns.WriteString(item.kind)
		_ = binary.Write(&icns, binary.BigEndian, uint32(asset.Len()+8))
		icns.Write(asset.Bytes())
	}
	data := icns.Bytes()
	binary.BigEndian.PutUint32(data[4:8], uint32(len(data)))
	return os.WriteFile(filepath.Join(out, name), data, 0o600)
}

func run(ctx context.Context, dir, program string, args ...string) error {
	cmd := proc.CommandContext(ctx, program, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func copyFile(from, to string, mode os.FileMode) error {
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
		return err
	}
	return os.WriteFile(to, b, mode)
}

func snapshot(ctx context.Context, source, out string, allowDirty bool) (string, bool, error) {
	cmd := proc.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = source
	b, err := cmd.Output()
	if err != nil {
		return "", false, err
	}
	commit := strings.TrimSpace(string(b))
	cmd = proc.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = source
	b, err = cmd.Output()
	if err != nil {
		return "", false, err
	}
	dirty := len(b) > 0
	if dirty && !allowDirty {
		return "", true, fmt.Errorf("source checkout is dirty; only explicit unsigned development snapshots allow dirty source")
	}
	if dirty {
		cmd = proc.CommandContext(ctx, "git", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
		cmd.Dir = source
		b, err = cmd.Output()
		if err != nil {
			return "", true, err
		}
		for _, name := range strings.Split(string(b), "\x00") {
			if name == "" {
				continue
			}
			if !filepath.IsLocal(name) {
				return "", true, fmt.Errorf("unsafe source path")
			}
			fi, err := os.Lstat(filepath.Join(source, name))
			if err != nil {
				return "", true, err
			}
			if !fi.Mode().IsRegular() {
				return "", true, fmt.Errorf("source symlinks are not supported")
			}
			if err := copyFile(filepath.Join(source, name), filepath.Join(out, name), fi.Mode().Perm()); err != nil {
				return "", true, err
			}
		}
		return commit, true, nil
	}
	cmd = proc.CommandContext(ctx, "git", "archive", "--format=tar", commit)
	cmd.Dir = source
	b, err = cmd.Output()
	if err != nil {
		return "", false, err
	}
	tr := tar.NewReader(bytes.NewReader(b))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false, err
		}
		if !filepath.IsLocal(h.Name) {
			return "", false, fmt.Errorf("unsafe archive path")
		}
		dest := filepath.Join(out, h.Name)
		if h.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(dest, 0o700); err != nil {
				return "", false, err
			}
			continue
		}
		if h.Typeflag != tar.TypeReg {
			return "", false, fmt.Errorf("source symlinks are not supported")
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return "", false, err
		}
		f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(h.Mode)&0o777)
		if err != nil {
			return "", false, err
		}
		_, err = io.Copy(f, tr)
		cerr := f.Close()
		if err != nil {
			return "", false, err
		}
		if cerr != nil {
			return "", false, cerr
		}
	}
	return commit, false, nil
}

func Build(ctx context.Context, o Options) (Result, error) {
	var result Result
	var pathErr error
	o.Output, pathErr = filepath.Abs(o.Output)
	if pathErr != nil {
		return result, pathErr
	}
	o.Source, pathErr = filepath.Abs(o.Source)
	if pathErr != nil {
		return result, pathErr
	}
	if !version.MatchString(o.Version) || o.BuildNumber < 1 || o.BuildNumber > 65535 {
		return result, fmt.Errorf("version must be x.y.z and build number must be 1–65535")
	}
	for _, part := range strings.Split(o.Version, ".") {
		if _, err := strconv.ParseUint(part, 10, 16); err != nil {
			return result, fmt.Errorf("version component exceeds Windows resource range")
		}
	}
	if o.Target != "darwin/arm64" && o.Target != "darwin/amd64" && o.Target != "windows/amd64" && o.Target != "windows/arm64" {
		return result, fmt.Errorf("unsupported enterprise target")
	}
	if o.AllowDirty && !o.Unsigned {
		return result, fmt.Errorf("dirty snapshots cannot be signed")
	}
	mac := strings.HasPrefix(o.Target, "darwin/")
	if mac && runtime.GOOS != "darwin" {
		return result, fmt.Errorf("macOS GUI builds require a macOS worker")
	}
	if !o.Unsigned && ((mac && (o.MacIdentity == "" || o.NotaryProfile == "")) || (!mac && (runtime.GOOS != "windows" || o.WindowsThumbprint == "" || o.TimestampURL == ""))) {
		return result, fmt.Errorf("production builds require the platform signing and notarization configuration")
	}
	prepared, err := os.MkdirTemp("", "enterprise-assets-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(prepared)
	p, err := Prepare(o.ProfilePath, o.LogoPath, prepared)
	if err != nil {
		return result, err
	}
	base := o.BaseRuntime
	if base == "" {
		base, err = os.MkdirTemp("", "enterprise-base-")
		if err != nil {
			return result, err
		}
		defer os.RemoveAll(base)
		runtimeOptions := o
		runtimeOptions.Output = base
		if _, err := BuildRuntime(ctx, runtimeOptions); err != nil {
			return result, err
		}
	}
	rt, err := ReadRuntime(base)
	if err != nil {
		return result, err
	}
	if rt.Target != o.Target || o.BaseSHA256 != "" && rt.SHA256 != o.BaseSHA256 {
		return result, fmt.Errorf("approved runtime changed or target differs")
	}
	if rt.Dirty && !o.Unsigned {
		return result, fmt.Errorf("dirty runtimes cannot be signed")
	}
	if err := emptyOutput(o.Output); err != nil {
		return result, err
	}
	logo, err := os.ReadFile(filepath.Join(prepared, "logo.png"))
	if err != nil {
		return result, err
	}
	config, err := json.Marshal(distribution.Package{SchemaVersion: 1, Profile: p, Version: o.Version, BuildNumber: o.BuildNumber, RuntimeVersion: rt.Version, Target: o.Target, Logo: logo})
	if err != nil {
		return result, err
	}
	if _, err := distribution.DecodePackage(config); err != nil {
		return result, err
	}
	work := base
	arch := strings.Split(o.Target, "/")[1]
	var artifact string
	if mac {
		bundle := filepath.Join(o.Output, p.Identity.ArtifactSlug+".app")
		binaryPath := filepath.Join(bundle, "Contents", "MacOS", p.Identity.ArtifactSlug)
		res := filepath.Join(bundle, "Contents", "Resources")
		if err := os.MkdirAll(res, 0o755); err != nil {
			return result, err
		}
		if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
			return result, err
		}
		if err := copyFile(filepath.Join(base, "runtime"), binaryPath, 0o755); err != nil {
			return result, err
		}
		if err := os.WriteFile(filepath.Join(res, distribution.PackageResource), config, 0o644); err != nil {
			return result, err
		}
		info, err := os.ReadFile(filepath.Join(base, "Info.plist"))
		if err != nil {
			return result, err
		}
		info, err = enterprisePlist(info, p, o.Version, o.BuildNumber)
		if err != nil {
			return result, err
		}
		if err := os.WriteFile(filepath.Join(bundle, "Contents", "Info.plist"), info, 0o644); err != nil {
			return result, err
		}
		if err := copyFile(filepath.Join(prepared, "enterprise.icns"), filepath.Join(res, "enterprise.icns"), 0o644); err != nil {
			return result, err
		}
		for _, name := range []string{"enterprise-dark.icns", "logo-light.png", "logo-dark.png", "app-light.png", "app-dark.png"} {
			if err := copyFile(filepath.Join(prepared, name), filepath.Join(res, name), 0o644); err != nil {
				return result, err
			}
		}
		if !o.Unsigned {
			if o.Progress != nil {
				o.Progress("signing")
			}
			for _, target := range []string{binaryPath, bundle} {
				if err := run(ctx, work, "codesign", "--force", "--options", "runtime", "--timestamp", "--sign", o.MacIdentity, target); err != nil {
					return result, err
				}
			}
			zip := filepath.Join(prepared, "notarize.zip")
			if err := run(ctx, work, "ditto", "-c", "-k", "--keepParent", bundle, zip); err != nil {
				return result, err
			}
			if err := run(ctx, work, "xcrun", "notarytool", "submit", zip, "--keychain-profile", o.NotaryProfile, "--wait"); err != nil {
				return result, err
			}
			if err := run(ctx, work, "xcrun", "stapler", "staple", bundle); err != nil {
				return result, err
			}
			if o.Progress != nil {
				o.Progress("verifying")
			}
			if err := run(ctx, work, "codesign", "--verify", "--deep", "--strict", bundle); err != nil {
				return result, err
			}
			if err := run(ctx, work, "spctl", "--assess", "--type", "execute", bundle); err != nil {
				return result, err
			}
		}
		artifact = p.Identity.ArtifactSlug + "-darwin-" + arch + ".zip"
		if err := run(ctx, work, "ditto", "-c", "-k", "--keepParent", bundle, filepath.Join(o.Output, artifact)); err != nil {
			return result, err
		}
	} else {
		artifact = p.Identity.ArtifactSlug + ".exe"
		if err := patchWindows(filepath.Join(base, "runtime"), filepath.Join(o.Output, artifact), prepared, p, o.Version, o.BuildNumber, config); err != nil {
			return result, err
		}
		if !o.Unsigned {
			if o.Progress != nil {
				o.Progress("signing")
			}
			if err := run(ctx, work, "signtool", "sign", "/sha1", o.WindowsThumbprint, "/fd", "SHA256", "/tr", o.TimestampURL, "/td", "SHA256", filepath.Join(o.Output, artifact)); err != nil {
				return result, err
			}
			if o.Progress != nil {
				o.Progress("verifying")
			}
			if err := run(ctx, work, "signtool", "verify", "/pa", filepath.Join(o.Output, artifact)); err != nil {
				return result, err
			}
		}
	}
	b, err := os.ReadFile(filepath.Join(o.Output, artifact))
	if err != nil {
		return result, err
	}
	sum := sha256.Sum256(b)
	result = Result{Profile: p, SourceCommit: rt.SourceCommit, Dirty: rt.Dirty, Version: o.Version, BuildNumber: o.BuildNumber, Target: o.Target, Signed: !o.Unsigned, RuntimeVersion: rt.Version, RuntimeSHA256: rt.SHA256, Assets: map[string]distribution.Asset{artifact: {Size: int64(len(b)), SHA256: hex.EncodeToString(sum[:])}}}
	metadata, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(filepath.Join(o.Output, "build-metadata.json"), append(metadata, '\n'), 0o600); err != nil {
		return result, err
	}
	return result, nil
}

func objectAt(root map[string]any, keys ...string) (map[string]any, error) {
	for _, key := range keys {
		var ok bool
		root, ok = root[key].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("resource template is missing object %s", key)
		}
	}
	return root, nil
}
