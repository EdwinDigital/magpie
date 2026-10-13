package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnterpriseDoesNotReplaceSharedCLI(t *testing.T) {
	old := cliDistribution
	cliDistribution = func() string { return "acme-assistant" }
	t.Cleanup(func() { cliDistribution = old })
	for _, shell := range []string{"", "zsh", "fish"} {
		if _, err := AddCLI(shell); err == nil || !strings.Contains(err.Error(), "shared magpie command") {
			t.Fatalf("enterprise CLI takeover was allowed for %q: %v", shell, err)
		}
	}
}

func TestEnterpriseCLIReportsOwnExecutableWithoutScanningPublicShells(t *testing.T) {
	oldID, oldExe, oldShells := cliDistribution, cliExe, cliShells
	t.Cleanup(func() { cliDistribution, cliExe, cliShells = oldID, oldExe, oldShells })
	cliDistribution = func() string { return "msai-client" }
	cliExe = func() (string, error) { return filepath.Join(t.TempDir(), "msai-client"), nil }
	cliShells = func() []cliShell { t.Fatal("enterprise scanned shells for public magpie"); return nil }
	cli := ReadCLI()
	if cli.Command != cli.Exe || cli.Stuck != "enterprise" || cli.Shim != "" || len(cli.Shells) != 0 {
		t.Fatalf("wrong enterprise CLI: %+v", cli)
	}
	// No alias to the public command is created as a side effect of inspection.
	if _, err := os.Stat(filepath.Join(filepath.Dir(cli.Exe), "magpie.cmd")); !os.IsNotExist(err) {
		t.Fatal("public shim was created")
	}
}
