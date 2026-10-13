package enterprisebuild

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"regexp"
	"strconv"

	"github.com/yetone/magpie/internal/distribution"
)

func enterprisePlist(in []byte, p distribution.Profile, version string, build int) ([]byte, error) {
	values := map[string]string{"CFBundleExecutable": p.Identity.ArtifactSlug, "CFBundleName": p.Brand.DisplayName, "CFBundleDisplayName": p.Brand.DisplayName, "CFBundleIdentifier": p.Identity.MacBundleID, "CFBundleIconFile": "enterprise", "CFBundleShortVersionString": version, "CFBundleVersion": strconv.Itoa(build), "CFBundleURLName": p.Identity.MacBundleID + ".import"}
	for k, v := range values {
		re := regexp.MustCompile(`(<key>` + regexp.QuoteMeta(k) + `</key>\s*<string>)[^<]*(</string>)`)
		if !re.Match(in) {
			return nil, fmt.Errorf("bundle template is missing %s", k)
		}
		in = re.ReplaceAllFunc(in, func(b []byte) []byte {
			m := re.FindSubmatch(b)
			return []byte(string(m[1]) + html.EscapeString(v) + string(m[2]))
		})
	}
	// Omit the original adaptive icon name; the enterprise .icns is its own.
	in = regexp.MustCompile(`<key>CFBundleIconName</key>\s*<string>[^<]*</string>`).ReplaceAll(in, nil)
	in = regexp.MustCompile(`(<key>CFBundleURLSchemes</key>\s*<array><string>)[^<]*(</string></array>)`).ReplaceAllFunc(in, func(b []byte) []byte {
		re := regexp.MustCompile(`(<key>CFBundleURLSchemes</key>\s*<array><string>)[^<]*(</string></array>)`)
		m := re.FindSubmatch(b)
		return []byte(string(m[1]) + html.EscapeString(p.Identity.URLScheme) + string(m[2]))
	})
	dec := xml.NewDecoder(bytes.NewReader(in))
	for {
		_, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}
	return in, nil
}
