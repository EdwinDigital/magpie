//go:build !windows

package update

import "context"

func verifyEnterpriseBinary(context.Context, string) error { return nil }
