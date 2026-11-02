package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// stubBinDir holds a built claudestub binary named "claude", used by execute() in place
// of the real CLI (see testdata/claudestub).
var stubBinDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "claudestub-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	name := "claude"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(dir, name)
	build := exec.Command("go", "build", "-o", out, "./testdata/claudestub")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "building claudestub:", err)
		os.Exit(1)
	}
	stubBinDir = dir
	os.Exit(m.Run())
}
