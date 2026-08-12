package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
)

func handleRoast(event *events.ApplicationCommandInteractionCreate) {
	if err := event.DeferCreateMessage(false); err != nil {
		return
	}

	data := event.SlashCommandInteractionData()
	target := data.User("user")

	var history string
	if chatLog != nil {
		msgs := chatLog.GetMessages(target.ID, 30*24*time.Hour)
		if len(msgs) > 0 {
			var trimmed []string
			total := 0
			for i := len(msgs) - 1; i >= 0; i-- {
				if total+len(msgs[i]) > 500 {
					break
				}
				trimmed = append([]string{msgs[i]}, trimmed...)
				total += len(msgs[i])
			}
			history = strings.Join(trimmed, "\n")
		}
	}

	roast, err := callAI(
		"Du bist ein Roast-Comedian. Kurze, bissige Einzeiler. Nie länger als EIN Satz. Deutsch.",
		buildRoastPrompt(target.EffectiveName(), history),
	)
	if err != nil {
		slog.Warn("roast AI failed", slog.Any("err", err))
		roast = fmt.Sprintf("%s ist heute leider zu langweilig für einen guten Röst.", target.Mention())
	}

	if _, err := event.Client().Rest.UpdateInteractionResponse(
		event.ApplicationID(),
		event.Token(),
		discord.MessageUpdate{Content: &roast},
	); err != nil {
		slog.Warn("failed to send roast", slog.Any("err", err))
	}
}

type chatRequest struct {
	Model       string        `json:"model"`
	Temperature float64       `json:"temperature"`
	Messages    []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

func buildRoastPrompt(name string, history string) string {
	if history == "" {
		return fmt.Sprintf("Röste @%s — wir wissen nichts über ihn/sie. Ein kurzer, bissiger Satz. Keine Erklärung, kein Aufbau, nur der Punch. Deutsch.\n\nGutes Beispiel: \"@Kevin — dein einziges Talent ist nicht aufzutauchen.\"", name)
	}
	return fmt.Sprintf("Chat von @%s:\n%s\n\nEin einziger kurzer, bissiger Satz der sich über DAS EINE lustigste Detail lustig macht. Keine Erklärung, kein Aufbau. Nur der Punch. Deutsch.\n\nGutes Beispiel: \"@Kevin — du hast 3x 'Pizza' geschrieben diese Woche. Dein Magen hat mehr Persönlichkeit als du.\"", name, history)
}

func callAI(system, prompt string) (string, error) {
	reqBody := chatRequest{
		Model:       aiModel,
		Temperature: 1.2,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: prompt},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, aiBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+aiAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("AI API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var cr chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", err
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("no choices in AI response")
	}

	return cr.Choices[0].Message.Content, nil
}
