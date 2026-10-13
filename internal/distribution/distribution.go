// Package distribution reads immutable package identity and defaults.
// Local preferences never choose an update identity.
package distribution

import (
	"crypto/ed25519"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

//go:embed profile.json assets/*
var resources embed.FS

type Defaults struct {
	// Nil inherits the six light-client defaults; an empty list selects none.
	VisibleAgents           []string `json:"visibleAgents"`
	VisibleProviders        []string `json:"visibleProviders"`
	ShowPluginProviderEntry bool     `json:"showPluginProviderEntry"`
	ShowCustomProviderEntry bool     `json:"showCustomProviderEntry"`
	PublicPluginCatalog     bool     `json:"publicPluginCatalog"`
	LibraryEnabled          bool     `json:"libraryEnabled"`
}

type Profile struct {
	SchemaVersion   int    `json:"schemaVersion"`
	TenantID        string `json:"tenantId"`
	DistributionID  string `json:"distributionId"`
	ProfileRevision int    `json:"profileRevision"`
	Brand           struct {
		DisplayName string `json:"displayName"`
		CompanyName string `json:"companyName"`
		LogoAsset   string `json:"logoAsset"`
	} `json:"brand"`
	Identity struct {
		MacBundleID  string `json:"macBundleId"`
		WindowsAppID string `json:"windowsAppId"`
		ArtifactSlug string `json:"artifactSlug"`
		URLScheme    string `json:"urlScheme"`
	} `json:"identity"`
	EnterpriseDefaults Defaults `json:"enterpriseDefaults"`
	Updates            struct {
		Mode             string            `json:"mode"`
		Channel          string            `json:"channel"`
		FeedURL          string            `json:"feedUrl"`
		ManifestKeys     map[string]string `json:"manifestKeys,omitempty"`
		WindowsPublisher string            `json:"windowsPublisher,omitempty"`
	} `json:"updates"`
}

var slug = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)
var identity = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*(\.[A-Za-z0-9-]+)+$`)
var logoID = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func ValidateName(name string) error {
	if name != strings.TrimSpace(name) || !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 64 {
		return fmt.Errorf("application name must have 1–64 characters without surrounding whitespace")
	}
	for _, c := range name {
		if unicode.IsControl(c) {
			return fmt.Errorf("application name contains a control character")
		}
	}
	return nil
}

func Validate(p Profile) error {
	if p.DistributionID == "" {
		return fmt.Errorf("distribution identity is required")
	}
	if p.SchemaVersion != 1 || p.ProfileRevision < 1 || !slug.MatchString(p.DistributionID) || !slug.MatchString(p.TenantID) || !slug.MatchString(p.Identity.ArtifactSlug) {
		return fmt.Errorf("invalid enterprise profile version or identity")
	}
	if err := ValidateName(p.Brand.DisplayName); err != nil {
		return err
	}
	if !identity.MatchString(p.Identity.MacBundleID) || !identity.MatchString(p.Identity.WindowsAppID) || !slug.MatchString(p.Identity.URLScheme) {
		return fmt.Errorf("invalid platform identity")
	}
	if reservedName(p.Identity.ArtifactSlug) || p.Identity.URLScheme == "magpie" || p.Identity.MacBundleID == "com.yetone.magpie" {
		return fmt.Errorf("enterprise identity must not claim a public or reserved application name")
	}
	if !logoID.MatchString(p.Brand.LogoAsset) {
		return fmt.Errorf("logo must be a SHA-256 asset")
	}
	if p.Updates.Mode != "manual" && p.Updates.Mode != "platform" {
		return fmt.Errorf("update mode must be manual or platform")
	}
	if p.Updates.Mode == "platform" {
		u, err := url.Parse(p.Updates.FeedURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || !slug.MatchString(p.Updates.Channel) || len(p.Updates.ManifestKeys) == 0 {
			return fmt.Errorf("enterprise updates need an HTTPS feed, channel and trusted keys")
		}
	}
	for _, key := range p.Updates.ManifestKeys {
		b, err := base64.StdEncoding.DecodeString(key)
		if err != nil || len(b) != ed25519.PublicKeySize {
			return fmt.Errorf("invalid trusted update public key")
		}
	}
	for _, id := range p.EnterpriseDefaults.VisibleProviders {
		if !ValidProviderID(id) {
			return fmt.Errorf("invalid provider selection %q", id)
		}
	}
	for _, id := range p.EnterpriseDefaults.VisibleAgents {
		if !ValidAgentID(id) {
			return fmt.Errorf("invalid agent selection %q", id)
		}
	}
	return nil
}

func reservedName(name string) bool {
	name = strings.ToLower(name)
	return name == "magpie" || name == "con" || name == "prn" || name == "aux" || name == "nul" ||
		regexp.MustCompile(`^(com|lpt)[1-9]$`).MatchString(name)
}

func ValidProviderID(id string) bool {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 || parts[1] == "" || len(id) > 256 || strings.ContainsAny(parts[1], "\x00\r\n/\\") {
		return false
	}
	switch parts[0] {
	case "subscription", "preset", "plugin", "custom":
		return true
	}
	return false
}

var current = func() Profile {
	if resourcePackage == "1" {
		b, err := packageResource()
		if err != nil {
			panic(fmt.Errorf("enterprise package: %w", err))
		}
		c, err := DecodePackage(b)
		if err != nil {
			panic(fmt.Errorf("enterprise package: %w", err))
		}
		if c.Target != runtime.GOOS+"/"+runtime.GOARCH {
			panic("enterprise package target does not match this runtime")
		}
		packageConfig = c
		return c.Profile
	}
	b, err := resources.ReadFile("profile.json")
	if err != nil {
		panic(err)
	}
	var p Profile
	if err := json.Unmarshal(b, &p); err != nil {
		panic(fmt.Errorf("enterprise profile: %w", err))
	}
	if p.DistributionID != "" {
		if err := Validate(p); err != nil {
			panic(err)
		}
		logo, err := resources.ReadFile("assets/logo.png")
		if err != nil || AssetID(logo) != p.Brand.LogoAsset {
			panic("enterprise logo does not match its profile")
		}
	}
	return p
}()

func Current() Profile {
	p := current
	p.EnterpriseDefaults.VisibleProviders = slices.Clone(p.EnterpriseDefaults.VisibleProviders)
	p.EnterpriseDefaults.VisibleAgents = slices.Clone(p.EnterpriseDefaults.VisibleAgents)
	p.Updates.ManifestKeys = maps.Clone(p.Updates.ManifestKeys)
	return p
}

func DefaultAgentIDs() []string {
	return []string{"codex", "claude", "claude-desktop", "vscode", "copilot", "workbuddy"}
}

func ValidAgentID(id string) bool {
	return id != "" && len(id) <= 256 && strings.TrimSpace(id) == id &&
		id != "." && id != ".." && !strings.ContainsAny(id, "/\\\x00\r\n\t")
}
func ID() string { return current.DistributionID }

// WiringID is a package-stable namespace in external agents' configurations.
// It does not change when the user edits the application's display name.
func WiringID() string {
	if ID() != "" {
		return "magpie-enterprise-" + ID()
	}
	return "magpie"
}

func WiringToken() string {
	if ID() != "" {
		// The delimiter prevents an id that prefixes another id from owning
		// the other's token (for example acme and acme-client).
		return WiringID() + "~"
	}
	return "magpie"
}

func GatewayPort() int {
	if ID() == "" {
		return 3425
	}
	hash := sha256.Sum256([]byte(ID()))
	return 35000 + (int(hash[0])*256+int(hash[1]))%15000
}
func Scheme() string {
	if ID() != "" {
		return current.Identity.URLScheme
	}
	return "magpie"
}
func Name() string {
	if ID() != "" {
		return current.Brand.DisplayName
	}
	return "magpie"
}
func BundleName() string {
	if ID() != "" {
		return current.Identity.ArtifactSlug + ".app"
	}
	return "magpie.app"
}
func AssetID(b []byte) string { sum := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(sum[:]) }
func Logo() []byte {
	if ID() == "" {
		return nil
	}
	if resourcePackage == "1" {
		return slices.Clone(packageConfig.Logo)
	}
	b, _ := resources.ReadFile("assets/logo.png")
	return b
}
