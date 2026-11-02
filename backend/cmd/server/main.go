package main

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"kanban_board/internal/api"
	"kanban_board/internal/store"
	"kanban_board/web"
)

func dataDir() string {
	if d := os.Getenv("KB_DATA"); d != "" {
		return d
	}
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "kanban-board")
}

// loadToken returns the persisted access token, creating it on first run.
func loadToken(dir string) string {
	if t := os.Getenv("KB_TOKEN"); t != "" {
		return t
	}
	f := filepath.Join(dir, "token")
	if b, err := os.ReadFile(f); err == nil && len(strings.TrimSpace(string(b))) >= 16 {
		return strings.TrimSpace(string(b))
	}
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	t := hex.EncodeToString(buf)
	if err := os.WriteFile(f, []byte(t), 0o600); err != nil {
		log.Fatal(err)
	}
	return t
}

// kbBinary locates the CLI (KB_BIN, or kb.exe next to the server binary).
func kbBinary() string {
	if p := os.Getenv("KB_BIN"); p != "" {
		return p
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	name := "kb"
	if runtime.GOOS == "windows" {
		name = "kb.exe"
	}
	p := filepath.Join(filepath.Dir(exe), name)
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

func lanIPs() []string {
	var out []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		// 169.254.x.x is an auto-assigned address from a disconnected adapter: never reachable
		if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && !ipn.IP.IsLinkLocalUnicast() && ipn.IP.To4() != nil {
			out = append(out, ipn.IP.String())
		}
	}
	return out
}

func main() {
	dir := dataDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "kanban.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	_ = st.MarkInterruptedRuns()

	token := loadToken(dir)
	srv, _ := api.New(st, dir, token, web.FS())

	addr := os.Getenv("KB_ADDR")
	if addr == "" {
		addr = ":7777"
	}
	_, port, _ := net.SplitHostPort(addr)
	if bin := kbBinary(); bin != "" {
		srv.ConfigureMCP(bin, "http://127.0.0.1:"+port)
		log.Printf("mcp:    runs usam %s mcp", bin)
	} else {
		log.Printf("mcp:    kb.exe não encontrado ao lado do servidor; execuções sem acesso ao board")
	}
	log.Printf("dados em %s", dir)
	log.Printf("local:  http://localhost:%s/?token=%s", port, token)
	for _, ip := range lanIPs() {
		log.Printf("rede:   http://%s:%s/?token=%s", ip, port, token)
	}
	log.Fatal(http.ListenAndServe(addr, srv.Handler()))
}
