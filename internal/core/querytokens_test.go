package core_test

import (
	"reflect"
	"testing"

	"github.com/kindbrave/claude-knowledger/internal/core"
)

func TestTokenizeQuery(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"only spaces", "   \t\n", nil},
		{"only punctuation", ",.;:!?", nil},
		{"single ascii token", "alpha", []string{"alpha"}},
		{"two ascii tokens", "a b", []string{"a", "b"}},
		{"multiple spaces collapse", "a   b\t\tc", []string{"a", "b", "c"}},
		{"fullwidth space", "语义　召回", []string{"语义", "召回"}},
		{"ascii punctuation", "hello,world.foo", []string{"hello", "world", "foo"}},
		{"cjk punctuation", "语义，召回。检索", []string{"语义", "召回", "检索"}},
		{"underscore preserved", "user_id", []string{"user_id"}},
		{"hyphen splits", "user-id", []string{"user", "id"}},
		{"case preserved", "Foo Bar", []string{"Foo", "Bar"}},
		{"dedup keeps first occurrence order", "a b a c b", []string{"a", "b", "c"}},
		{"mixed cjk and ascii", "alpha 语义 beta", []string{"alpha", "语义", "beta"}},
		{"digits stay", "v1 v2", []string{"v1", "v2"}},
		{"asterisk splits", "a*b", []string{"a", "b"}},
		{"colon splits", "a:b", []string{"a", "b"}},
		{"caret splits", "a^b", []string{"a", "b"}},
		{"tilde splits", "a~b", []string{"a", "b"}},
		{"plus splits", "a+b", []string{"a", "b"}},
		{"equals splits", "a=b", []string{"a", "b"}},
		{"pipe splits", "a|b", []string{"a", "b"}},
		{"dollar splits", "$a$b", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := core.TokenizeQuery(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("TokenizeQuery(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}
