//go:build !darwin && !windows

package distribution

import "fmt"

func packageResource() ([]byte, error) {
	return nil, fmt.Errorf("enterprise runtime unsupported on this platform")
}
