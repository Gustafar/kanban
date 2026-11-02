// Package selfupdate rebuilds and restarts the installed app (install.ps1) when a card that changes
// the app itself reaches review.
package selfupdate

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"kanban_board/internal/proc"
)

// Root returns the repo the running server was installed from (<root>\backend\bin\kanban-server.exe), or "".
func Root() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if _, err := os.Stat(filepath.Join(root, "install.ps1")); err != nil {
		return ""
	}
	return root
}

// SamePath reports whether two filesystem paths point at the same folder.
func SamePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// Install runs root's install.ps1 detached, building from source (the card's worktree, or root itself).
// The script stops this very server, so the child must not depend on us.
func Install(root, source, logDir string) {
	if runtime.GOOS != "windows" {
		return
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-File", filepath.Join(root, "install.ps1"), "-Source", source, "-NoOpen")
	cmd.Dir = root
	if f, err := os.Create(filepath.Join(logDir, "install.log")); err == nil {
		cmd.Stdout, cmd.Stderr = f, f
		defer f.Close() // the child keeps its own handle
	}
	proc.Detach(cmd)
	if err := cmd.Start(); err != nil {
		log.Printf("auto-install: %v", err)
		return
	}
	log.Printf("auto-install: compilando %s", source)
	_ = cmd.Process.Release()
}
