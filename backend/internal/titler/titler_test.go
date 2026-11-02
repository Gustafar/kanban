package titler

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		name, raw, title string
		tags             int
		wantErr          bool
	}{
		{"plain", `{"result":"{\"title\":\"Corrigir paginação\",\"tags\":[\"bug\",\"frontend\"]}"}`, "Corrigir paginação", 2, false},
		{"fenced", "{\"result\":\"```json\\n{\\\"title\\\":\\\"Exportar XLSX.\\\",\\\"tags\\\":[]}\\n```\"}", "Exportar XLSX", 0, false},
		{"no json", `{"result":"não sei"}`, "", 0, true},
		{"empty title", `{"result":"{\"title\":\" \",\"tags\":[]}"}`, "", 0, true},
		{"cli error", `{"is_error":true,"result":"limite atingido"}`, "", 0, true},
		{"garbage", `nope`, "", 0, true},
	}
	for _, c := range cases {
		title, tags, err := parse([]byte(c.raw))
		if (err != nil) != c.wantErr {
			t.Fatalf("%s: err=%v wantErr=%v", c.name, err, c.wantErr)
		}
		if err == nil && (title != c.title || len(tags) != c.tags) {
			t.Fatalf("%s: got %q %v", c.name, title, tags)
		}
	}
}
