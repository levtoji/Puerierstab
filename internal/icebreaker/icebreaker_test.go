package icebreaker

import (
	"strings"
	"testing"
)

func TestDefaultQuestionsNotEmpty(t *testing.T) {
	questions := defaultQuestions()
	if len(questions) == 0 {
		t.Fatalf("expected non-empty default questions")
	}
	for i, q := range questions {
		if strings.TrimSpace(q) == "" {
			t.Fatalf("question %d is empty", i)
		}
	}
}

func TestLoadQuestionsFromEnv(t *testing.T) {
	t.Setenv(icebreakerConfigEnv, `["Was ist dein Lieblingsessen?", "Wo warst du zuletzt im Urlaub?"]`)
	questions, err := loadQuestions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(questions) != 2 {
		t.Fatalf("expected 2 questions, got %d", len(questions))
	}
	if questions[0] != "Was ist dein Lieblingsessen?" {
		t.Fatalf("expected first question, got %q", questions[0])
	}
}

func TestLoadQuestionsEmptyEnv(t *testing.T) {
	t.Setenv(icebreakerConfigEnv, "")
	questions, err := loadQuestions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(questions) == 0 {
		t.Fatalf("expected default questions when env is empty")
	}
}

func TestLoadQuestionsInvalidJSON(t *testing.T) {
	t.Setenv(icebreakerConfigEnv, "not-json")
	_, err := loadQuestions()
	if err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
}

func TestLoadQuestionsEmptyArray(t *testing.T) {
	t.Setenv(icebreakerConfigEnv, "[]")
	questions, err := loadQuestions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(questions) == 0 {
		t.Fatalf("expected default questions when array is empty")
	}
}

func TestPickRandom(t *testing.T) {
	questions := []string{"A", "B", "C"}
	pick1 := pickRandom(questions)
	pick2 := pickRandom(questions)

	if pick1 < 0 || pick1 > 2 {
		t.Fatalf("pick out of range: %d", pick1)
	}

	_ = pick2
}

func TestQuestionEmbed(t *testing.T) {
	embed := questionEmbed("Warum ist die Banane krumm?")
	if embed.Title != "Frage des Tages" {
		t.Fatalf("expected title 'Frage des Tages', got %q", embed.Title)
	}
	if embed.Description != "Warum ist die Banane krumm?" {
		t.Fatalf("expected description, got %q", embed.Description)
	}
	if embed.Color != 0x5865F2 {
		t.Fatalf("expected color 0x5865F2, got 0x%X", embed.Color)
	}
}
