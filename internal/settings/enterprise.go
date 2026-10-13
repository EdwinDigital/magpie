package settings

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/yetone/magpie/internal/distribution"
)

type Enterprise struct {
	SchemaVersion  int      `json:"schemaVersion"`
	OverrideFields []string `json:"overrideFields"`
	AppName        string   `json:"appName"`
	LogoAsset      string   `json:"logoAsset"`
	distribution.Defaults
}

var EnterpriseFields = []string{"appName", "logoAsset", "visibleProviders", "visibleAgents", "showPluginProviderEntry", "showCustomProviderEntry", "publicPluginCatalog", "libraryEnabled"}
var ErrEnterpriseConflict = errors.New("enterprise configuration changed in another window; reload before saving")

func EnterpriseDefaults() Enterprise {
	p := distribution.Current()
	e := Enterprise{SchemaVersion: 1, AppName: distribution.Name(), LogoAsset: p.Brand.LogoAsset, Defaults: p.EnterpriseDefaults}
	if e.VisibleAgents == nil {
		e.VisibleAgents = distribution.DefaultAgentIDs()
	}
	return e
}

func ResolveEnterprise(s Settings) Enterprise {
	return resolveEnterprise(s, EnterpriseDefaults())
}

func resolveEnterprise(s Settings, e Enterprise) Enterprise {
	if s.Enterprise == nil {
		return e
	}
	if ValidateEnterprise(s.Enterprise) != nil {
		return e
	}
	in := *s.Enterprise
	fields := in.OverrideFields
	if fields == nil {
		fields = EnterpriseFields
	}
	for _, f := range fields {
		switch f {
		case "appName":
			if in.AppName != "" {
				e.AppName = in.AppName
			}
		case "logoAsset":
			if in.LogoAsset != "" {
				e.LogoAsset = in.LogoAsset
			}
		case "visibleProviders":
			e.VisibleProviders = slices.Clone(in.VisibleProviders)
		case "visibleAgents":
			if in.VisibleAgents != nil {
				e.VisibleAgents = slices.Clone(in.VisibleAgents)
			}
		case "showPluginProviderEntry":
			e.ShowPluginProviderEntry = in.ShowPluginProviderEntry
		case "showCustomProviderEntry":
			e.ShowCustomProviderEntry = in.ShowCustomProviderEntry
		case "publicPluginCatalog":
			e.PublicPluginCatalog = in.PublicPluginCatalog
		case "libraryEnabled":
			e.LibraryEnabled = in.LibraryEnabled
		}
	}
	e.OverrideFields = slices.Clone(fields)
	return e
}

func EnterpriseRevision(s Settings) string {
	b, _ := json.Marshal(struct {
		Local   *Enterprise
		Profile distribution.Profile
	}{s.Enterprise, distribution.Current()})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func ValidateEnterprise(in *Enterprise) error {
	if in == nil {
		return nil
	}
	if in.SchemaVersion != 1 {
		return fmt.Errorf("unsupported enterprise configuration version")
	}
	if in.AppName != "" {
		if err := distribution.ValidateName(in.AppName); err != nil {
			return err
		}
	}
	if in.LogoAsset != "" && (len(in.LogoAsset) != 71 || !strings.HasPrefix(in.LogoAsset, "sha256:")) {
		return fmt.Errorf("invalid logo asset")
	}
	if in.LogoAsset != "" {
		if _, err := hex.DecodeString(strings.TrimPrefix(in.LogoAsset, "sha256:")); err != nil {
			return fmt.Errorf("invalid logo asset")
		}
	}
	for _, f := range in.OverrideFields {
		if !slices.Contains(EnterpriseFields, f) {
			return fmt.Errorf("unknown enterprise field %q", f)
		}
	}
	if len(in.VisibleProviders) > 1000 {
		return fmt.Errorf("too many provider selections")
	}
	for _, id := range in.VisibleProviders {
		if !distribution.ValidProviderID(id) {
			return fmt.Errorf("invalid provider selection %q", id)
		}
	}
	if len(in.VisibleAgents) > 1000 {
		return fmt.Errorf("too many agent selections")
	}
	for _, id := range in.VisibleAgents {
		if !distribution.ValidAgentID(id) {
			return fmt.Errorf("invalid agent selection %q", id)
		}
	}
	return nil
}

// SetEnterprise compares and updates under the same lock. Ordinary writers
// preserve this field even when their settings snapshot predates the change.
func SetEnterprise(in *Enterprise, revision string) error {
	fileMu.Lock()
	defer fileMu.Unlock()
	s := load()
	if revision != EnterpriseRevision(s) {
		return ErrEnterpriseConflict
	}
	if err := ValidateEnterprise(in); err != nil {
		return err
	}
	if in != nil {
		copy := *in
		copy.VisibleProviders = slices.Clone(in.VisibleProviders)
		slices.Sort(copy.VisibleProviders)
		copy.VisibleProviders = slices.Compact(copy.VisibleProviders)
		copy.VisibleAgents = slices.Clone(in.VisibleAgents)
		slices.Sort(copy.VisibleAgents)
		copy.VisibleAgents = slices.Compact(copy.VisibleAgents)
		copy.OverrideFields = slices.Clone(in.OverrideFields)
		s.Enterprise = &copy
	} else {
		s.Enterprise = nil
	}
	return saveEnterprise(s, false, true)
}

func EnterpriseAllowed(id string, e Enterprise) bool {
	return id == "subscription:copilot" || id == "preset:ollama" || slices.Contains(e.VisibleProviders, id)
}

func EnterpriseAgentAllowed(id string, e Enterprise) bool {
	if slices.Contains(e.VisibleAgents, id) {
		return true
	}
	// WSL twins and named profiles inherit their base agent's selection.
	base, _, _ := strings.Cut(id, "@wsl:")
	base, _, _ = strings.Cut(base, "#")
	return slices.Contains(e.VisibleAgents, base)
}

// Automatic discovery is restricted in enterprise packages and explicit local
// enterprise configurations. A generic CLI without either keeps its behavior.
func AgentDiscoveryAllowed(id string) bool {
	s := Load()
	return distribution.ID() == "" && s.Enterprise == nil || EnterpriseAgentAllowed(id, ResolveEnterprise(s))
}

func ProviderDiscoveryAllowed(id string) bool {
	s := Load()
	return distribution.ID() == "" && s.Enterprise == nil || EnterpriseAllowed(id, ResolveEnterprise(s))
}
