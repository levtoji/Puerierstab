package asciireact

import "testing"

func TestMatchKeyword(t *testing.T) {
	r := New()
	tests := []struct {
		content string
		want    bool
	}{
		{"ich mag hunde", true},
		{"HUNDE sind toll", true},
		{"die Katze schläft", true},
		{"MiaU!", true},
		{"party heute?", true},
		{"Montag morgen", true},
		{"Pizza ist lecker", true},
		{"kaffee bitte", true},
		{"keine relevanz", false},
		{"", false},
	}

	for _, tt := range tests {
		_, ok := r.matchKeyword(tt.content)
		if ok != tt.want {
			t.Fatalf("matchKeyword(%q) = %v, want %v", tt.content, ok, tt.want)
		}
	}
}

func TestNoMatchEmpty(t *testing.T) {
	r := New()
	_, ok := r.matchKeyword("xyz 123 nichts")
	if ok {
		t.Fatal("expected no match")
	}
}
