package plugin

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	MaxZIPSize     = 32 << 20
	maxZIPExpanded = 128 << 20
	maxZIPFileSize = 32 << 20
	maxZIPEntries  = 4096
)

// AddZIP imports a ready-to-load package, then uses the local-folder installer.
// Local plugins load in place, so successful imports must outlive the upload.
func AddZIP(ctx context.Context, r io.ReaderAt, size int64) (Entry, error) {
	if size <= 0 || size > MaxZIPSize {
		return Entry{}, errors.New("plugin ZIP must be at most 32 MiB")
	}
	z, err := zip.NewReader(r, size)
	if err != nil {
		return Entry{}, fmt.Errorf("invalid plugin ZIP: %w", err)
	}
	if len(z.File) == 0 || len(z.File) > maxZIPEntries {
		return Entry{}, errors.New("plugin ZIP must contain 1 to 4096 entries")
	}
	imports := filepath.Join(Dir(), "imports")
	if err := os.MkdirAll(imports, 0o700); err != nil {
		return Entry{}, err
	}
	dir, err := os.MkdirTemp(imports, "zip-")
	if err != nil {
		return Entry{}, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(dir)
		}
	}()
	unpacked := filepath.Join(dir, ".unpacked")
	if err := os.Mkdir(unpacked, 0o700); err != nil {
		return Entry{}, err
	}
	if err := extractZIP(ctx, z, unpacked); err != nil {
		return Entry{}, err
	}
	root, name, err := zipPackageRoot(unpacked)
	if err != nil {
		return Entry{}, err
	}
	target := filepath.Join(dir, name)
	if err := os.Rename(root, target); err != nil {
		return Entry{}, err
	}
	if root != unpacked {
		_ = os.RemoveAll(unpacked)
	}
	e, err := addEntry(ctx, target, "", filepath.Base(dir))
	if err != nil {
		return Entry{}, err
	}
	keep = true
	return e, nil
}

func extractZIP(ctx context.Context, z *zip.Reader, dir string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	seen := map[string]bool{}
	var total int64
	for _, f := range z.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := strings.TrimSuffix(f.Name, "/")
		if err := zipPath(name); err != nil {
			return err
		}
		mode := f.Mode()
		if !mode.IsRegular() && !mode.IsDir() {
			return fmt.Errorf("plugin ZIP contains a link or special file: %s", name)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("plugin ZIP contains duplicate paths: %s", name)
		}
		seen[key] = true
		if f.UncompressedSize64 > maxZIPFileSize || f.UncompressedSize64 > uint64(maxZIPExpanded-total) {
			return errors.New("plugin ZIP exceeds 32 MiB per file or 128 MiB extracted")
		}
		// Finder metadata is not part of the package or its wrapper directory.
		if strings.Split(name, "/")[0] == "__MACOSX" || path.Base(name) == ".DS_Store" {
			continue
		}
		if mode.IsDir() {
			if err := root.MkdirAll(name, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := root.MkdirAll(path.Dir(name), 0o700); err != nil {
			return err
		}
		out, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600|mode.Perm()&0o100)
		if err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		limit := min(int64(maxZIPFileSize), int64(maxZIPExpanded)-total)
		n, copyErr := io.Copy(out, io.LimitReader(&zipContextReader{ctx, in}, limit+1))
		closeErr := out.Close()
		in.Close()
		if copyErr != nil {
			return fmt.Errorf("could not extract %s: %w", name, copyErr)
		}
		if closeErr != nil {
			return closeErr
		}
		if n > limit {
			return errors.New("plugin ZIP exceeds 32 MiB per file or 128 MiB extracted")
		}
		total += n
	}
	return nil
}

type zipContextReader struct {
	ctx context.Context
	in  io.Reader
}

func (r *zipContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.in.Read(p)
}

// Validate for every desktop, not only the OS processing the upload. This also
// rejects aliases such as Windows reserved names and trailing dots/spaces.
func zipPath(name string) error {
	bad := func() error { return fmt.Errorf("unsafe path in plugin ZIP: %q", name) }
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:<>\"|?*") {
		return bad()
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, ". ") != part {
			return bad()
		}
		for _, c := range part {
			if c < 32 || c == 127 {
				return bad()
			}
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return bad()
		}
	}
	return nil
}

func zipPackageRoot(dir string) (string, string, error) {
	root := dir
	if _, err := os.Stat(filepath.Join(root, "package.json")); os.IsNotExist(err) {
		es, err := os.ReadDir(root)
		if err != nil || len(es) != 1 || !es[0].IsDir() {
			return "", "", errors.New("plugin ZIP needs package.json at its root or in one enclosing folder")
		}
		root = filepath.Join(root, es[0].Name())
	}
	b, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return "", "", errors.New("plugin ZIP needs package.json at its root or in one enclosing folder")
	}
	var pkg struct {
		Name string `json:"name"`
		Main string `json:"main"`
	}
	if !strings.HasPrefix(strings.TrimSpace(string(b)), "{") || json.Unmarshal(b, &pkg) != nil {
		return "", "", errors.New("plugin ZIP contains an invalid package.json")
	}
	if err := notPlugin(root); err != nil {
		return "", "", err
	}
	pkg.Main = strings.TrimSpace(pkg.Main)
	if pkg.Main != "" {
		if err := zipPath(pkg.Main); err != nil {
			// Node packages commonly spell their entry as ./dist/index.js.
			if !strings.HasPrefix(pkg.Main, "./") || zipPath(strings.TrimPrefix(pkg.Main, "./")) != nil {
				return "", "", errors.New("plugin ZIP has an invalid main entry")
			}
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(pkg.Main))); err != nil {
			return "", "", errors.New("plugin ZIP is missing its main entry; include the built plugin files")
		}
	}
	name := "plugin"
	if pkgName.MatchString(pkg.Name) {
		name = path.Base(pkg.Name)
	}
	if err := zipPath(name); err != nil {
		return "", "", err
	}
	return root, name, nil
}

func removeArchive(e Entry) {
	if e.Archive == "" || !strings.HasPrefix(e.Archive, "zip-") || filepath.Base(e.Archive) != e.Archive {
		return
	}
	dir := filepath.Join(Dir(), "imports", e.Archive)
	// Never treat an ordinary local folder as an owned archive import.
	if filepath.Dir(Target(e.Spec)) == dir {
		_ = os.RemoveAll(dir)
	}
}
