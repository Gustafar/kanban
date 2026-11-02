package runner

import (
	"reflect"
	"testing"

	"kanban_board/internal/store"
)

func TestModelArgs(t *testing.T) {
	cases := []struct {
		name string
		card store.Card
		want []string
	}{
		{"defaults add nothing", store.Card{}, nil},
		{"model only", store.Card{Model: "opus"}, []string{"--model", "opus"}},
		{"effort only", store.Card{Effort: "high"}, []string{"--effort", "high"}},
		{"both", store.Card{Model: "sonnet", Effort: "low"}, []string{"--model", "sonnet", "--effort", "low"}},
	}
	for _, c := range cases {
		if got := modelArgs(c.card); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
