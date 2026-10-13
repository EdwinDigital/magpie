//go:build windows

package update

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/yetone/magpie/internal/distribution"
	"github.com/yetone/magpie/internal/proc"
)

func verifyEnterpriseBinary(ctx context.Context, path string) error {
	if distribution.ID() == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	want := distribution.Current().Updates.WindowsPublisher
	if want == "" {
		return fmt.Errorf("enterprise Windows publisher is not configured")
	}
	// The path is data in an environment variable, never PowerShell source.
	cmd := proc.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `$ErrorActionPreference='Stop'; $s=Get-AuthenticodeSignature -LiteralPath $env:MAGPIE_VERIFY_PATH; @{status=$s.Status.ToString();subject=$s.SignerCertificate.Subject}|ConvertTo-Json -Compress`)
	cmd.Env = append(os.Environ(), "MAGPIE_VERIFY_PATH="+path)
	b, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("enterprise binary signature: %w", err)
	}
	var result struct {
		Status  string `json:"status"`
		Subject string `json:"subject"`
	}
	if json.Unmarshal(b, &result) != nil || result.Status != "Valid" || result.Subject != want {
		return fmt.Errorf("enterprise binary signature or publisher is not trusted")
	}
	return nil
}
