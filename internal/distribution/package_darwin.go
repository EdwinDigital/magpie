package distribution

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func packageResource() ([]byte, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(exe)
	if filepath.Base(dir) != "MacOS" || filepath.Base(filepath.Dir(dir)) != "Contents" {
		return nil, fmt.Errorf("enterprise runtime must run inside its application bundle")
	}
	f, err := os.Open(filepath.Join(filepath.Dir(dir), "Resources", PackageResource))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, MaxPackageSize+1))
}
