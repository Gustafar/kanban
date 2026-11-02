// Command claudestub stands in for the real `claude` CLI in runner tests: it records the
// arguments it was called with and emits a minimal stream-json transcript on stdout.
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	if out := os.Getenv("CLAUDE_STUB_ARGS_FILE"); out != "" {
		_ = os.WriteFile(out, []byte(strings.Join(os.Args[1:], "\x00")), 0o644)
	}
	session := os.Getenv("CLAUDE_STUB_SESSION")
	if session == "" {
		session = "stub-session"
	}
	result := os.Getenv("CLAUDE_STUB_RESULT")
	if result == "" {
		result = "ok"
	}
	fmt.Printf(`{"type":"system","session_id":%q}`+"\n", session)
	fmt.Printf(`{"type":"result","is_error":false,"result":%q}`+"\n", result)
}
