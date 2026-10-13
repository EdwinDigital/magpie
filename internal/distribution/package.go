package distribution

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Only a reusable enterprise runtime reads package resources. Missing data
// fails closed, rather than launching a public client and sharing its state.
var resourcePackage string
var packageConfig Package

const PackageResource = "enterprise-runtime.json"
const MaxPackageSize = 4 << 20

type Package struct {
	SchemaVersion  int     `json:"schemaVersion"`
	Profile        Profile `json:"profile"`
	Version        string  `json:"version"`
	BuildNumber    int     `json:"buildNumber"`
	RuntimeVersion string  `json:"runtimeVersion"`
	Target         string  `json:"target"`
	Logo           []byte  `json:"logo"`
}

func ValidateVersion(v string, build int) error {
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(v) || build < 1 || build > 65535 {
		return fmt.Errorf("version must be x.y.z and build number must be 1-65535")
	}
	for _, part := range strings.Split(v, ".") {
		if _, err := strconv.ParseUint(part, 10, 16); err != nil {
			return fmt.Errorf("version component exceeds platform resource range")
		}
	}
	return nil
}

func DecodePackage(b []byte) (Package, error) {
	var c Package
	if len(b) > MaxPackageSize {
		return c, fmt.Errorf("enterprise package configuration is too large")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, err
	}
	if d.Decode(new(any)) != io.EOF {
		return c, fmt.Errorf("one package configuration is required")
	}
	if c.SchemaVersion != 1 || c.RuntimeVersion == "" {
		return c, fmt.Errorf("invalid package schema or runtime version")
	}
	if err := ValidateVersion(c.Version, c.BuildNumber); err != nil {
		return c, err
	}
	if err := Validate(c.Profile); err != nil {
		return c, err
	}
	if len(c.Logo) > 2<<20 || AssetID(c.Logo) != c.Profile.Brand.LogoAsset {
		return c, fmt.Errorf("enterprise logo does not match its profile")
	}
	img, err := png.DecodeConfig(bytes.NewReader(c.Logo))
	if err != nil || img.Width < 1 || img.Height < 1 || img.Width > 4_000_000/img.Height {
		return c, fmt.Errorf("invalid enterprise PNG logo")
	}
	switch c.Target {
	case "darwin/arm64", "darwin/amd64", "windows/arm64", "windows/amd64":
	default:
		return c, fmt.Errorf("invalid enterprise target")
	}
	return c, nil
}

func PackageVersion(fallback string) string {
	if resourcePackage == "1" {
		return packageConfig.Version
	}
	return fallback
}

func ExecutableName() string {
	if ID() != "" {
		return current.Identity.ArtifactSlug
	}
	return "magpie"
}

// The display name may be Unicode; the artifact slug is the process/CLI name.
func CLIHelp(text string) string {
	text = strings.Replace(text, "magpie —", Name()+" —", 1)
	if ID() != "" {
		text = strings.ReplaceAll(text, "magpie ", ExecutableName()+" ")
	}
	return text
}
