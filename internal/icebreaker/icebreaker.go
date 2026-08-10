package icebreaker

import (
	"encoding/json"
	"log/slog"
	"math/rand"
	"os"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
)

const icebreakerConfigEnv = "ICE_BREAKER_QUESTIONS_JSON"

func defaultQuestions() []string {
	return []string{
		"Wenn du einen Tag lang mit einer fiktiven Figur tauschen könntest, welche wäre es?",
		"Welcher Film hat dich zuletzt so richtig überrascht?",
		"Was ist dein ultimatives Comfort-Food?",
		"Welches Spiel würdest du jedem empfehlen, der noch nie gezockt hat?",
		"Wenn du eine Superkraft für den Alltag haben könntest, welche?",
		"Was war dein peinlichster Autokorrektur-Moment?",
		"Welche Serie hast du zuletzt gebinged?",
		"Wenn Geld keine Rolle spielen würde, was würdest du beruflich machen?",
		"Was ist eine unpopuläre Meinung, die du vertrittst?",
		"Welches Land steht ganz oben auf deiner Reiseliste?",
	}
}

func loadQuestions() ([]string, error) {
	raw := strings.TrimSpace(os.Getenv(icebreakerConfigEnv))
	if raw == "" {
		return defaultQuestions(), nil
	}
	var questions []string
	if err := json.Unmarshal([]byte(raw), &questions); err != nil {
		return nil, err
	}
	if len(questions) == 0 {
		return defaultQuestions(), nil
	}
	return questions, nil
}

func pickRandom(questions []string) int {
	return rand.Intn(len(questions))
}

func questionEmbed(question string) discord.Embed {
	return discord.Embed{
		Title:       "Frage des Tages",
		Description: question,
		Color:       0x5865F2,
	}
}

type Handler struct {
	questions []string
}

func NewHandler() (*Handler, error) {
	questions, err := loadQuestions()
	if err != nil {
		return nil, err
	}
	return &Handler{questions: questions}, nil
}

func (h *Handler) OnCommand(event *events.ApplicationCommandInteractionCreate) {
	if err := event.DeferCreateMessage(false); err != nil {
		return
	}

	idx := pickRandom(h.questions)
	embed := questionEmbed(h.questions[idx])

	if _, err := event.Client().Rest.UpdateInteractionResponse(
		event.ApplicationID(),
		event.Token(),
		discord.NewMessageUpdate().WithEmbeds(embed),
	); err != nil {
		slog.Warn("failed to post icebreaker", slog.Any("err", err))
	}
}
