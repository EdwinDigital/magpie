package enterprisebuild

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tc-hib/winres"
	winversion "github.com/tc-hib/winres/version"
	"github.com/yetone/magpie/internal/distribution"
)

// Windows customization changes PE resources, not machine code. Authenticode
// signing happens afterwards and covers the frozen profile/logo as RCDATA too.
func patchWindows(base, out, prepared string, p distribution.Profile, version string, build int, config []byte) error {
	in, err := os.Open(base)
	if err != nil {
		return err
	}
	defer in.Close()
	rs, err := winres.LoadFromEXE(in)
	if err != nil {
		return err
	}
	if err := rs.Set(winres.RT_RCDATA, winres.Name("ENTERPRISE_PACKAGE"), 0, config); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(prepared, "app-light.png"))
	if err != nil {
		return err
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return err
	}
	icon, err := winres.NewIconFromResizedImage(img, []int{16, 32, 64, 128, 256})
	if err != nil {
		return err
	}
	if err := rs.SetIcon(winres.Name("APP"), icon); err != nil {
		return err
	}
	v, err := winversion.FromBytes(rs.Get(winres.RT_VERSION, winres.ID(1), winres.LCIDDefault))
	if err != nil {
		return fmt.Errorf("invalid runtime version resource: %w", err)
	}
	for i, part := range strings.Split(version, ".") {
		n, _ := strconv.ParseUint(part, 10, 16)
		v.FileVersion[i] = uint16(n)
	}
	v.FileVersion[3] = uint16(build)
	v.ProductVersion = v.FileVersion
	for k, value := range map[string]string{winversion.ProductName: p.Brand.DisplayName, winversion.FileDescription: p.Brand.DisplayName, winversion.CompanyName: p.Brand.CompanyName, winversion.OriginalFilename: p.Identity.ArtifactSlug + ".exe", winversion.InternalName: p.Identity.ArtifactSlug, winversion.FileVersion: version, winversion.ProductVersion: version} {
		if err := v.Set(winversion.LangDefault, k, value); err != nil {
			return err
		}
	}
	rs.SetVersionInfo(*v)
	manifest, err := winres.AppManifestFromXML(rs.Get(winres.RT_MANIFEST, winres.ID(1), winres.LCIDDefault))
	if err != nil {
		return fmt.Errorf("invalid runtime manifest: %w", err)
	}
	manifest.Description = p.Brand.DisplayName
	manifest.Identity.Name = p.Identity.WindowsAppID
	manifest.Identity.Version = v.FileVersion
	rs.SetManifest(manifest)
	f, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	err = rs.WriteToEXE(f, in)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
