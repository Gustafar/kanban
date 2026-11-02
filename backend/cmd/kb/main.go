package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const usage = `cardume (kb) - CLI do board

  cardume [caminho]                        abre o board no projeto da pasta atual (cria o projeto se não existir)
  kb projects                              lista projetos
  kb project-add <nome> [caminho]          cria projeto (caminho padrão: pasta atual)
  kb ls [projeto]                          lista cards (projeto por nome ou id)
  kb add <projeto> <título> [-d descrição] [--auto]   cria card no backlog
  kb move <card> <backlog|todo|doing|review|done>     move card (doing dispara a IA)
  kb run <card> [mensagem]                 executa (ou continua) a IA no card
  kb approve <card>                        aprova execução pendente
  kb cancel <card>                         cancela execução
  kb logs <card>                           mostra o log da última execução
  kb diff <card>                           mostra o diff da última execução
  kb merge <card>                          faz merge da branch do card
  kb mcp                                   servidor MCP (stdio) para o Claude usar o board
  kb open                                  abre o board no navegador, sem escolher projeto

env: KB_URL (padrão http://localhost:7777), KB_TOKEN`

var base = envOr("KB_URL", "http://localhost:7777")

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func token() string {
	if t := os.Getenv("KB_TOKEN"); t != "" {
		return t
	}
	dir, _ := os.UserConfigDir()
	b, _ := os.ReadFile(filepath.Join(dir, "kanban-board", "token"))
	return strings.TrimSpace(string(b))
}

// api talks to the board server and returns errors instead of exiting (the MCP server needs that).
func api(method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, base+path, rd)
	req.Header.Set("Authorization", "Bearer "+token())
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("servidor indisponível em %s (está rodando?): %v", base, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var e struct{ Error string }
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return fmt.Errorf("erro: %s", e.Error)
		}
		return fmt.Errorf("erro %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		_ = json.Unmarshal(data, out)
	}
	return nil
}

func call(method, path string, body any, out any) {
	if err := api(method, path, body, out); err != nil {
		die("%v", err)
	}
}

func die(f string, a ...any) {
	fmt.Fprintf(os.Stderr, f+"\n", a...)
	os.Exit(1)
}

type project struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}
type card struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"projectId"`
	Title     string `json:"title"`
	Column    string `json:"column"`
	Mode      string `json:"mode"`
	RunStatus string `json:"runStatus"`
}
type run struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}
type event struct {
	Kind string `json:"kind"`
	Data string `json:"data"`
}

func findProject(ref string) project {
	var ps []project
	call("GET", "/api/projects", nil, &ps)
	id, _ := strconv.ParseInt(ref, 10, 64)
	for _, p := range ps {
		if p.ID == id || strings.EqualFold(p.Name, ref) {
			return p
		}
	}
	die("projeto %q não encontrado", ref)
	return project{}
}

func cardID(s string) int64 {
	id, err := strconv.ParseInt(strings.TrimPrefix(s, "#"), 10, 64)
	if err != nil {
		die("id de card inválido: %s", s)
	}
	return id
}

func latestRun(cid int64) run {
	var rs []run
	call("GET", fmt.Sprintf("/api/cards/%d/runs", cid), nil, &rs)
	if len(rs) == 0 {
		die("o card #%d ainda não tem execuções", cid)
	}
	return rs[0]
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		openFolder("")
		return
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "help", "-h", "--help":
		fmt.Println(usage)
	case "mcp":
		serveMCP()
	case "open":
		url := base + "/?token=" + token()
		openBrowser(url)
		fmt.Println("abrindo", base)
	case "projects":
		var ps []project
		call("GET", "/api/projects", nil, &ps)
		for _, p := range ps {
			fmt.Printf("%3d  %-20s %s\n", p.ID, p.Name, p.Path)
		}
	case "project-add":
		if len(rest) < 1 {
			die("uso: kb project-add <nome> [caminho]")
		}
		path, _ := os.Getwd()
		if len(rest) > 1 {
			path, _ = filepath.Abs(rest[1])
		}
		var p project
		call("POST", "/api/projects", map[string]string{"name": rest[0], "path": path}, &p)
		fmt.Printf("projeto #%d criado: %s\n", p.ID, p.Path)
	case "ls":
		var ps []project
		if len(rest) > 0 {
			ps = []project{findProject(rest[0])}
		} else {
			call("GET", "/api/projects", nil, &ps)
		}
		for _, p := range ps {
			var cs []card
			call("GET", fmt.Sprintf("/api/projects/%d/cards", p.ID), nil, &cs)
			fmt.Printf("== %s ==\n", p.Name)
			for _, c := range cs {
				st := ""
				if c.RunStatus != "" {
					st = " [" + c.RunStatus + "]"
				}
				fmt.Printf("  #%-4d %-8s %s%s\n", c.ID, c.Column, c.Title, st)
			}
		}
	case "add":
		if len(rest) < 2 {
			die("uso: kb add <projeto> <título> [-d descrição] [--auto]")
		}
		p := findProject(rest[0])
		body := map[string]string{"title": rest[1], "column": "backlog", "mode": "approval"}
		for i := 2; i < len(rest); i++ {
			switch rest[i] {
			case "-d":
				if i+1 < len(rest) {
					i++
					body["description"] = rest[i]
				}
			case "--auto":
				body["mode"] = "auto"
			}
		}
		var c card
		call("POST", fmt.Sprintf("/api/projects/%d/cards", p.ID), body, &c)
		fmt.Printf("card #%d criado\n", c.ID)
	case "move":
		if len(rest) < 2 {
			die("uso: kb move <card> <coluna>")
		}
		call("POST", fmt.Sprintf("/api/cards/%d/move", cardID(rest[0])), map[string]any{"column": rest[1], "position": float64(time.Now().UnixMilli())}, nil)
		fmt.Println("ok")
	case "run":
		if len(rest) < 1 {
			die("uso: kb run <card> [mensagem]")
		}
		call("POST", fmt.Sprintf("/api/cards/%d/run", cardID(rest[0])), map[string]string{"message": strings.Join(rest[1:], " ")}, nil)
		fmt.Println("execução solicitada")
	case "approve", "cancel":
		if len(rest) < 1 {
			die("uso: kb %s <card>", cmd)
		}
		r := latestRun(cardID(rest[0]))
		call("POST", fmt.Sprintf("/api/runs/%d/%s", r.ID, cmd), nil, nil)
		fmt.Println("ok")
	case "logs":
		if len(rest) < 1 {
			die("uso: kb logs <card>")
		}
		r := latestRun(cardID(rest[0]))
		var es []event
		call("GET", fmt.Sprintf("/api/runs/%d/events", r.ID), nil, &es)
		fmt.Printf("# execução %d: %s\n", r.ID, r.Status)
		for _, e := range es {
			fmt.Printf("[%s] %s\n", e.Kind, e.Data)
		}
	case "diff":
		if len(rest) < 1 {
			die("uso: kb diff <card>")
		}
		r := latestRun(cardID(rest[0]))
		var d struct{ Diff string }
		call("GET", fmt.Sprintf("/api/runs/%d/diff", r.ID), nil, &d)
		fmt.Println(d.Diff)
	case "merge":
		if len(rest) < 1 {
			die("uso: kb merge <card>")
		}
		r := latestRun(cardID(rest[0]))
		call("POST", fmt.Sprintf("/api/runs/%d/merge", r.ID), nil, nil)
		fmt.Println("merge feito")
	default:
		if fi, err := os.Stat(cmd); err == nil && fi.IsDir() {
			openFolder(cmd)
			return
		}
		fmt.Println(usage)
	}
}

func openBrowser(url string) {
	if err := exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start(); err != nil {
		die("não consegui abrir o navegador: %v", err)
	}
}

// folderRoot resolves dir to its git root when it has one, otherwise to dir itself.
func folderRoot(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		die("caminho inválido: %v", err)
	}
	if out, err := exec.Command("git", "-C", abs, "rev-parse", "--show-toplevel").Output(); err == nil {
		if top := strings.TrimSpace(string(out)); top != "" {
			return filepath.Clean(top)
		}
	}
	return abs
}

// within reports whether path is dir or lives under it (case-insensitive, as on Windows).
func within(path, dir string) bool {
	path, dir = strings.ToLower(filepath.Clean(path)), strings.ToLower(filepath.Clean(dir))
	return path == dir || strings.HasPrefix(path, strings.TrimRight(dir, `\/`)+string(filepath.Separator))
}

// openFolder opens the board on the project that owns dir (default: the current folder), registering it first if needed.
func openFolder(dir string) {
	if dir == "" {
		dir = "."
	}
	abs, _ := filepath.Abs(dir)
	var ps []project
	call("GET", "/api/projects", nil, &ps)
	var found *project
	for i, p := range ps {
		if within(abs, p.Path) && (found == nil || len(p.Path) > len(found.Path)) {
			found = &ps[i]
		}
	}
	if found == nil {
		root := folderRoot(abs)
		name := filepath.Base(root)
		var p project
		call("POST", "/api/projects", map[string]string{"name": name, "path": root}, &p)
		found = &p
		fmt.Printf("projeto %q criado em %s\n", p.Name, p.Path)
	}
	fmt.Printf("abrindo %s\n", found.Name)
	openBrowser(fmt.Sprintf("%s/?token=%s&project=%d", base, token(), found.ID))
}
