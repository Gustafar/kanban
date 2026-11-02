package runner

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"kanban_board/internal/proc"
)

// run executes a build/test command in dir and returns its combined output.
func run(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	proc.Hide(cmd)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CI=1") // keep test runners (npm/jest/etc.) from waiting on input or watching files
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Verify runs the target project's own build/test commands, detected from its root files.
// It reports the combined output and whether everything it ran succeeded; a project with
// no recognized build system (no go.mod, no package.json) always reports ok.
func Verify(dir string) (output string, ok bool) {
	var sections []string
	ok = true

	step := func(label string, name string, args ...string) {
		out, err := run(dir, name, args...)
		if err != nil {
			ok = false
			label += " (falhou)"
		} else {
			label += " (ok)"
		}
		if out != "" {
			sections = append(sections, label+":\n"+out)
		} else {
			sections = append(sections, label)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		step("go build ./...", "go", "build", "./...")
		if ok {
			step("go vet ./...", "go", "vet", "./...")
		}
	} else if b, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if json.Unmarshal(b, &pkg) == nil {
			if _, has := pkg.Scripts["build"]; has {
				step("npm run build", "npm", "run", "build")
			}
			if _, has := pkg.Scripts["test"]; has && ok {
				step("npm test", "npm", "test")
			}
		}
	}

	return strings.Join(sections, "\n\n"), ok
}
