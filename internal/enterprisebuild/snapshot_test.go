package enterprisebuild

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanGitSnapshotAcceptsMetadataButRejectsLinks(t *testing.T) {
	for _, link := range []bool{false, true} {
		name := "regular"
		if link {
			name = "symlink"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			source, out := filepath.Join(root, "source"), filepath.Join(root, "snapshot")
			if err := os.MkdirAll(filepath.Join(source, "nested"), 0o700); err != nil {
				t.Fatal(err)
			}
			body := []byte("tracked source bytes\n")
			if err := os.WriteFile(filepath.Join(source, "nested", "source.txt"), body, 0o600); err != nil {
				t.Fatal(err)
			}
			if link {
				if err := os.Symlink("../outside.txt", filepath.Join(source, "escape.txt")); err != nil {
					t.Skipf("symlink fixture unavailable: %v", err)
				}
			}
			ctx := context.Background()
			for _, args := range [][]string{
				{"init", "--quiet"}, {"config", "user.name", "Snapshot Test"},
				{"config", "user.email", "snapshot@example.invalid"}, {"add", "."},
				{"commit", "--quiet", "-m", "snapshot fixture"},
			} {
				if err := run(ctx, source, "git", args...); err != nil {
					t.Fatal(err)
				}
			}
			commit, dirty, err := snapshot(ctx, source, out, false)
			if link {
				if err == nil || !strings.Contains(err.Error(), "symlinks") {
					t.Fatalf("source link accepted: %v", err)
				}
				return
			}
			if err != nil || dirty || len(commit) != 40 {
				t.Fatalf("clean Git archive rejected: commit=%q dirty=%v err=%v", commit, dirty, err)
			}
			if got, err := os.ReadFile(filepath.Join(out, "nested", "source.txt")); err != nil || string(got) != string(body) {
				t.Fatalf("source bytes changed: %q, %v", got, err)
			}
			if _, err := os.Stat(filepath.Join(out, "pax_global_header")); !os.IsNotExist(err) {
				t.Fatalf("Git metadata was extracted as a source file: %v", err)
			}
		})
	}
}
