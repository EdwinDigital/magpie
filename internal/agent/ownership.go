package agent

import (
	"crypto/sha256"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/gateway"
)

const publicDesktopProfileID = "00000000-0000-4000-8000-6d6167706965"

func wiringProfileID(id string) string {
	if id == "magpie" {
		return publicDesktopProfileID
	}
	s := sha256.Sum256([]byte(id))
	s[6], s[8] = s[6]&0x0f|0x50, s[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", s[:4], s[4:6], s[6:8], s[8:10], s[10:16])
}

func wiringNamespace(v string) bool {
	return v == "magpie" || strings.HasPrefix(v, "magpie-enterprise-")
}

func keyOwner(key string) string {
	key = strings.TrimPrefix(key, "Bearer ")
	if strings.HasPrefix(key, "magpie-enterprise-") {
		if gateway.Token != "magpie" && (key == gateway.Token || strings.HasPrefix(key, gateway.Token+"-")) {
			return magpieID
		}
		return "another enterprise client"
	}
	if key == "magpie" || strings.HasPrefix(key, "magpie-") && !strings.HasPrefix(key, "magpie-enterprise-") {
		return "magpie"
	}
	return ""
}

func endpointOwner(base, own string) string {
	if base == "" {
		return ""
	}
	u, err := url.Parse(base)
	v, ownErr := url.Parse(own)
	if err != nil || ownErr != nil {
		return ""
	}
	loopback := func(host string) bool {
		if strings.EqualFold(host, "localhost") {
			return true
		}
		ip := net.ParseIP(host)
		return ip != nil && ip.IsLoopback()
	}
	if u.Host != "" && v.Host != "" && u.Scheme == v.Scheme && u.Port() == v.Port() && (strings.EqualFold(u.Hostname(), v.Hostname()) || loopback(u.Hostname()) && loopback(v.Hostname())) {
		return magpieID
	}
	if loopback(u.Hostname()) {
		if u.Port() == "3425" {
			return "magpie"
		}
		return "another local gateway"
	}
	return ""
}

// Old enterprise releases used public markers. They can be converted only
// on an explicit operation, with both this private record and its own address.
func ownedLegacyWiring(agentID, base string) bool {
	return magpieID != "magpie" && base != "" && endpointOwner(base, gateway.URL()) == magpieID && len(appliedOf(agentID).Fields) > 0
}

// ForeignOwner describes configuration that belongs to another client, not a
// setting in this client's own applied.json. Inactive historical tables are
// ignored by adapters that provide an active-owner reader.
func (a *Agent) ForeignOwner() string {
	if a.WiringOwner != nil {
		if owner := a.WiringOwner(); owner != "" && owner != magpieID {
			return owner
		}
		return ""
	}
	if a.Path == "" {
		return ""
	}
	b, err := edit.Read(a.Path)
	if err != nil {
		return ""
	}
	for _, kv := range keyValues(b) {
		if owner := keyOwner(kv.value); owner != "" && owner != magpieID {
			return owner
		}
	}
	return ""
}

func (a *Agent) wiringConflict() error {
	if owner := a.ForeignOwner(); owner != "" {
		return fmt.Errorf("%s is connected to another client (%s). Disconnect it there before connecting here", a.Name, owner)
	}
	return nil
}

func isolateWiring(a *Agent) {
	for i := range a.Fields {
		set := a.Fields[i].Set
		if set != nil {
			a.Fields[i].Set = func(v string) error {
				if err := a.wiringConflict(); err != nil {
					return err
				}
				return set(v)
			}
		}
	}
	if sync := a.Sync; sync != nil {
		a.Sync = func() error {
			if a.ForeignOwner() != "" {
				return nil
			}
			return sync()
		}
	}
	if join := a.Join; join != nil {
		a.Join = func() (bool, error) {
			if err := a.wiringConflict(); err != nil {
				return false, err
			}
			return join()
		}
	}
	if follow := a.Follow; follow != nil {
		a.Follow = func() error {
			if a.ForeignOwner() != "" {
				return nil
			}
			return follow()
		}
	}
}
