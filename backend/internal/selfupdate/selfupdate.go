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
// On success the script stops this very server before finishing, so onResult normally never runs; it
// only fires if the script fails before that point (e.g. the build broke), letting the caller surface
// the error to the user instead of leaving them waiting for a restart that will never happen.
func Install(root, source, logDir string, onResult func(err error)) {
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
		if onResult != nil {
			onResult(err)
		}
		return
	}
	log.Printf("auto-install: compilando %s", source)
	go func() {
		if err := cmd.Wait(); err != nil && onResult != nil {
			onResult(err)
		}
	}()
}

// LogTail returns the last maxBytes of install.log, to show the user why an update failed.
func LogTail(logDir string, maxBytes int) string {
	data, err := os.ReadFile(filepath.Join(logDir, "install.log"))
	if err != nil {
		return ""
	}
	if len(data) > maxBytes {
		data = data[len(data)-maxBytes:]
	}
	return strings.TrimSpace(string(data))
}
