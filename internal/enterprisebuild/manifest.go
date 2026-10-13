package enterprisebuild

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/yetone/magpie/internal/distribution"
)

func ReadPrivateKey(path string) (ed25519.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if block, _ := pem.Decode(b); block != nil {
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		if key, ok := key.(ed25519.PrivateKey); ok {
			return key, nil
		}
		return nil, fmt.Errorf("manifest signing needs an Ed25519 key")
	}
	if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b))); err == nil {
		b = decoded
	}
	if len(b) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid Ed25519 private key")
	}
	return ed25519.PrivateKey(b), nil
}

func SignManifest(result Result, dir, baseURL, keyID string, sequence uint64, key ed25519.PrivateKey) (distribution.Manifest, error) {
	m := distribution.Manifest{}
	if !result.Signed || result.Dirty {
		return m, fmt.Errorf("only verified production artifacts can be published")
	}
	if err := distribution.Validate(result.Profile); err != nil {
		return m, err
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return m, fmt.Errorf("download base URL must be HTTPS")
	}
	m = distribution.Manifest{SchemaVersion: 1, DistributionID: result.Profile.DistributionID, Channel: result.Profile.Updates.Channel, Sequence: sequence, Version: result.Version, ProfileRevision: result.Profile.ProfileRevision, BundleName: result.Profile.Identity.ArtifactSlug + ".app", Assets: map[string]distribution.Asset{}, KeyID: keyID}
	for name, asset := range result.Assets {
		if !filepath.IsLocal(name) || filepath.Base(name) != name {
			return m, fmt.Errorf("invalid artifact name")
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return m, err
		}
		sum := sha256.Sum256(b)
		if asset.SHA256 != hex.EncodeToString(sum[:]) || asset.Size != int64(len(b)) {
			return m, fmt.Errorf("artifact differs from build metadata")
		}
		asset.URL = strings.TrimRight(baseURL, "/") + "/" + url.PathEscape(name)
		key := name
		// Architecture remains part of the update lookup key; employees get
		// a plainly named EXE from a target-specific URL/directory.
		if strings.HasPrefix(result.Target, "windows/") && name == result.Profile.Identity.ArtifactSlug+".exe" {
			key = result.Profile.Identity.ArtifactSlug + "-" + strings.ReplaceAll(result.Target, "/", "-") + ".exe"
		}
		m.Assets[key] = asset
	}
	if len(key) != ed25519.PrivateKeySize {
		return m, fmt.Errorf("invalid signing key")
	}
	payload, err := m.SigningBytes()
	if err != nil {
		return m, err
	}
	m.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, payload))
	if err := distribution.VerifyManifest(m, result.Profile); err != nil {
		return m, err
	}
	return m, nil
}

func ReadResult(path string) (Result, error) {
	var r Result
	b, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(b, &r)
	return r, err
}
