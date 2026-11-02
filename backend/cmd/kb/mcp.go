package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Minimal MCP server over stdio (newline-delimited JSON-RPC 2.0): initialize, ping, tools/list, tools/call.
// KB_CARD_ID, when set (runs started by the board), becomes the default card for the card tools.

type rpcReq struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	run         func(args map[string]any) (string, error)
}

func schema(required []string, props map[string]any) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func argStr(a map[string]any, k string) string {
	v, _ := a[k].(string)
	return strings.TrimSpace(v)
}

// cardArg resolves card_id (number or "#12") or falls back to the card this run belongs to.
func cardArg(a map[string]any) (int64, error) {
	switch v := a["card_id"].(type) {
	case float64:
		return int64(v), nil
	case string:
		if id, err := strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(v), "#"), 10, 64); err == nil {
			return id, nil
		}
	}
	if id, err := strconv.ParseInt(os.Getenv("KB_CARD_ID"), 10, 64); err == nil {
		return id, nil
	}
	return 0, fmt.Errorf("informe card_id")
}

func projectByRef(ref string) (project, error) {
	var ps []project
	if err := api("GET", "/api/projects", nil, &ps); err != nil {
		return project{}, err
	}
	id, _ := strconv.ParseInt(ref, 10, 64)
	for _, p := range ps {
		if p.ID == id || strings.EqualFold(p.Name, ref) {
			return p, nil
		}
	}
	return project{}, fmt.Errorf("projeto %q não encontrado (use list_projects)", ref)
}

type fullCard struct {
	card
	Description string `json:"description"`
}

func getCard(id int64) (fullCard, error) {
	var c fullCard
	err := api("GET", fmt.Sprintf("/api/cards/%d", id), nil, &c)
	return c, err
}

func pretty(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

var columnEnum = "backlog | todo | review | done"

func tools() []tool {
	return []tool{
		{
			Name: "list_projects", Description: "Lista os projetos do quadro kanban (id, nome, pasta).",
			InputSchema: schema(nil, map[string]any{}),
			run: func(map[string]any) (string, error) {
				var ps []project
				if err := api("GET", "/api/projects", nil, &ps); err != nil {
					return "", err
				}
				return pretty(ps), nil
			},
		},
		{
			Name: "list_cards", Description: "Lista os cards de um projeto, com coluna e status da última execução.",
			InputSchema: schema([]string{"project"}, map[string]any{"project": str("nome ou id do projeto")}),
			run: func(a map[string]any) (string, error) {
				p, err := projectByRef(argStr(a, "project"))
				if err != nil {
					return "", err
				}
				var cs []card
				if err := api("GET", fmt.Sprintf("/api/projects/%d/cards", p.ID), nil, &cs); err != nil {
					return "", err
				}
				return pretty(cs), nil
			},
		},
		{
			Name: "get_card", Description: "Mostra um card completo (título, descrição, coluna, modo) com os comentários. Sem card_id, usa o card desta execução.",
			InputSchema: schema(nil, map[string]any{"card_id": str("id do card")}),
			run: func(a map[string]any) (string, error) {
				id, err := cardArg(a)
				if err != nil {
					return "", err
				}
				c, err := getCard(id)
				if err != nil {
					return "", err
				}
				var cs []struct {
					Author    string `json:"author"`
					Body      string `json:"body"`
					CreatedAt string `json:"createdAt"`
				}
				_ = api("GET", fmt.Sprintf("/api/cards/%d/comments", id), nil, &cs)
				return pretty(map[string]any{"card": c, "comments": cs}), nil
			},
		},
		{
			Name: "create_card", Description: "Cria um card em um projeto. Use para registrar subtarefas ou ideias encontradas durante o trabalho. Por padrão vai para o backlog.",
			InputSchema: schema([]string{"project", "title"}, map[string]any{
				"project":     str("nome ou id do projeto"),
				"title":       str("título curto"),
				"description": str("o que deve ser feito, com contexto suficiente para outra execução"),
				"column":      str("coluna: " + columnEnum + " (padrão backlog)"),
			}),
			run: func(a map[string]any) (string, error) {
				p, err := projectByRef(argStr(a, "project"))
				if err != nil {
					return "", err
				}
				col := argStr(a, "column")
				if col == "" {
					col = "backlog"
				}
				if col == "doing" {
					return "", fmt.Errorf("só o usuário pode iniciar execuções; crie o card em backlog ou todo")
				}
				var c card
				err = api("POST", fmt.Sprintf("/api/projects/%d/cards", p.ID), map[string]string{
					"title": argStr(a, "title"), "description": argStr(a, "description"), "column": col, "mode": "approval",
				}, &c)
				return fmt.Sprintf("card #%d criado em %s", c.ID, col), err
			},
		},
		{
			Name: "update_card", Description: "Altera título e/ou descrição de um card. Sem card_id, usa o card desta execução.",
			InputSchema: schema(nil, map[string]any{"card_id": str("id do card"), "title": str("novo título"), "description": str("nova descrição (substitui a atual)")}),
			run: func(a map[string]any) (string, error) {
				id, err := cardArg(a)
				if err != nil {
					return "", err
				}
				c, err := getCard(id)
				if err != nil {
					return "", err
				}
				title, desc := c.Title, c.Description
				if v := argStr(a, "title"); v != "" {
					title = v
				}
				if v, ok := a["description"].(string); ok {
					desc = v
				}
				err = api("PUT", fmt.Sprintf("/api/cards/%d", id), map[string]string{"title": title, "description": desc, "mode": c.Mode}, nil)
				return fmt.Sprintf("card #%d atualizado", id), err
			},
		},
		{
			Name: "add_note", Description: "Adiciona um comentário (autor: IA) ao card, para registrar progresso, decisões ou pendências. Sem card_id, usa o card desta execução.",
			InputSchema: schema([]string{"text"}, map[string]any{"card_id": str("id do card"), "text": str("texto do comentário")}),
			run: func(a map[string]any) (string, error) {
				id, err := cardArg(a)
				if err != nil {
					return "", err
				}
				text := argStr(a, "text")
				if text == "" {
					return "", fmt.Errorf("text é obrigatório")
				}
				err = api("POST", fmt.Sprintf("/api/cards/%d/comments", id), map[string]string{"body": text, "author": "ai"}, nil)
				return "comentário adicionado", err
			},
		},
		{
			Name: "move_card", Description: "Move um card entre colunas (" + columnEnum + "). Não é possível mover para 'doing': só o usuário inicia execuções. Sem card_id, usa o card desta execução.",
			InputSchema: schema([]string{"column"}, map[string]any{"card_id": str("id do card"), "column": str("coluna de destino: " + columnEnum)}),
			run: func(a map[string]any) (string, error) {
				id, err := cardArg(a)
				if err != nil {
					return "", err
				}
				col := argStr(a, "column")
				if col == "doing" {
					return "", fmt.Errorf("só o usuário pode iniciar execuções (coluna doing)")
				}
				err = api("POST", fmt.Sprintf("/api/cards/%d/move", id), map[string]any{"column": col, "position": float64(time.Now().UnixMilli())}, nil)
				return fmt.Sprintf("card #%d movido para %s", id, col), err
			},
		},
	}
}

func serveMCP() {
	ts := tools()
	byName := map[string]tool{}
	for _, t := range ts {
		byName[t.Name] = t
	}
	out := json.NewEncoder(os.Stdout)
	reply := func(id json.RawMessage, result any, rpcErr map[string]any) {
		m := map[string]any{"jsonrpc": "2.0", "id": id}
		if rpcErr != nil {
			m["error"] = rpcErr
		} else {
			m["result"] = result
		}
		_ = out.Encode(m)
	}

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		var req rpcReq
		if json.Unmarshal(sc.Bytes(), &req) != nil || req.Method == "" {
			continue
		}
		isNotification := len(req.ID) == 0
		switch req.Method {
		case "initialize":
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if p.ProtocolVersion == "" {
				p.ProtocolVersion = "2025-06-18"
			}
			reply(req.ID, map[string]any{
				"protocolVersion": p.ProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "kanban", "version": "0.1.0"},
			}, nil)
		case "ping":
			reply(req.ID, map[string]any{}, nil)
		case "tools/list":
			reply(req.ID, map[string]any{"tools": ts}, nil)
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			t, ok := byName[p.Name]
			if !ok {
				reply(req.ID, nil, map[string]any{"code": -32602, "message": "ferramenta desconhecida: " + p.Name})
				continue
			}
			if p.Arguments == nil {
				p.Arguments = map[string]any{}
			}
			text, err := t.run(p.Arguments)
			if err != nil {
				reply(req.ID, map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": err.Error()}}}, nil)
			} else {
				reply(req.ID, map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}, nil)
			}
		default:
			if !isNotification {
				reply(req.ID, nil, map[string]any{"code": -32601, "message": "método não suportado: " + req.Method})
			}
		}
	}
}
