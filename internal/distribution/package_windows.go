package distribution

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func packageResource() ([]byte, error) {
	res, err := windows.FindResource(0, "ENTERPRISE_PACKAGE", windows.RT_RCDATA)
	if err != nil {
		return nil, fmt.Errorf("missing enterprise resource: %w", err)
	}
	size, err := windows.SizeofResource(0, res)
	if err != nil || size == 0 || size > MaxPackageSize {
		return nil, fmt.Errorf("invalid enterprise resource size")
	}
	b, err := windows.LoadResourceData(0, res)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), b...), nil
}
