package update

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/yetone/magpie/internal/distribution"
	"github.com/yetone/magpie/internal/edit"
)

var enterpriseSequenceMu sync.Mutex

func artifactURL(distributionID, mirror, asset string) string {
	if distributionID != "" {
		return asset
	}
	return mirrored(mirror, asset)
}

func enterpriseReleaseFor(body io.Reader, profile distribution.Profile, path string) (*Release, error) {
	var m distribution.Manifest
	dec := json.NewDecoder(io.LimitReader(body, 2<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	if dec.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("one update manifest is required")
	}
	if err := distribution.VerifyManifest(m, profile); err != nil {
		return nil, err
	}
	if parse(m.Version) == nil {
		return nil, fmt.Errorf("enterprise update has no release version")
	}
	enterpriseSequenceMu.Lock()
	defer enterpriseSequenceMu.Unlock()
	var sequence uint64
	if b, err := os.ReadFile(path); err == nil {
		var perr error
		sequence, perr = strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
		if perr != nil {
			return nil, fmt.Errorf("cannot read enterprise update sequence")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if m.Sequence < sequence {
		return nil, fmt.Errorf("enterprise update replay rejected")
	}
	if m.Sequence > sequence {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		if err := edit.WriteAtomic(path, []byte(strconv.FormatUint(m.Sequence, 10)+"\n")); err != nil {
			return nil, err
		}
	}
	r := &Release{Version: m.Version, Notes: m.Notes, URL: m.URL, Assets: map[string]Asset{}}
	for k, a := range m.Assets {
		r.Assets[k] = Asset{URL: a.URL, Size: a.Size, SHA256: a.SHA256}
	}
	return r, nil
}

func enterpriseBundleIdentity(app string) error {
	if distribution.ID() == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		return err
	}
	dec := xml.NewDecoder(bytes.NewReader(b))
	key := ""
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if start, ok := tok.(xml.StartElement); ok {
			if start.Name.Local == "key" {
				if err := dec.DecodeElement(&key, &start); err != nil {
					return err
				}
			}
			if start.Name.Local == "string" && key == "CFBundleIdentifier" {
				var id string
				if err := dec.DecodeElement(&id, &start); err != nil {
					return err
				}
				if id != distribution.Current().Identity.MacBundleID {
					return fmt.Errorf("enterprise bundle identity changed")
				}
				return nil
			}
		}
	}
	return fmt.Errorf("enterprise bundle identity missing")
}
