package distribution

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
)

type Asset struct {
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest's canonical signed payload is json.Marshal of this type with an
// empty Signature. Assets' map keys are sorted by encoding/json.
type Manifest struct {
	SchemaVersion   int              `json:"schemaVersion"`
	DistributionID  string           `json:"distributionId"`
	Channel         string           `json:"channel"`
	Sequence        uint64           `json:"sequence"`
	Version         string           `json:"version"`
	ProfileRevision int              `json:"profileRevision"`
	BundleName      string           `json:"bundleName"`
	Notes           string           `json:"notes"`
	URL             string           `json:"url"`
	Assets          map[string]Asset `json:"assets"`
	KeyID           string           `json:"keyId"`
	Signature       string           `json:"signature"`
}

func (m Manifest) SigningBytes() ([]byte, error) { m.Signature = ""; return json.Marshal(m) }

func VerifyManifest(m Manifest, p Profile) error {
	if m.SchemaVersion != 1 || m.DistributionID != p.DistributionID || m.Channel != p.Updates.Channel || m.Sequence == 0 || m.ProfileRevision < 1 || m.Version == "" || m.BundleName != p.Identity.ArtifactSlug+".app" {
		return fmt.Errorf("update manifest has the wrong enterprise identity, channel or version")
	}
	pub, err := base64.StdEncoding.DecodeString(p.Updates.ManifestKeys[m.KeyID])
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("unknown update signing key")
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("invalid update signature")
	}
	payload, err := m.SigningBytes()
	if err != nil || !ed25519.Verify(pub, payload, sig) {
		return fmt.Errorf("update signature verification failed")
	}
	if len(m.Assets) == 0 {
		return fmt.Errorf("update has no assets")
	}
	if m.URL != "" {
		u, err := url.Parse(m.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return fmt.Errorf("invalid signed release page")
		}
	}
	for _, a := range m.Assets {
		u, err := url.Parse(a.URL)
		hash, herr := hex.DecodeString(a.SHA256)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || herr != nil || len(hash) != 32 || a.Size <= 0 {
			return fmt.Errorf("invalid signed update asset")
		}
	}
	return nil
}
