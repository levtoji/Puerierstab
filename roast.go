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
		"Du schreibst witzige, bissige Einzeiler. Kurz und pointiert. Deutsch.",
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
		return fmt.Sprintf("Mach einen witzigen Einzeiler über jemanden namens @%s von dem wir absolut nichts wissen. Das ist der Witz — wir wissen nichts. Ein Satz. Deutsch.\n\nGutes Beispiel: \"@Kevin — selbst Siri sagt 'keine Ergebnisse' wenn sie nach deiner Persönlichkeit sucht.\"", name)
	}
	return fmt.Sprintf("Chat von @%s:\n%s\n\nEin einziger kurzer, bissiger Satz der sich über DAS EINE lustigste Detail lustig macht. Keine Erklärung, kein Aufbau. Nur der Punch. Deutsch.\n\nGutes Beispiel: \"@Kevin — du hast 3x 'Pizza' geschrieben diese Woche. Dein Magen hat mehr Persönlichkeit als du.\"", name, history)
}

func callAI(system, prompt string) (string, error) {
	return callAIWithModel(system, prompt, aiModel)
}

func callAIWithModel(system, prompt, model string) (string, error) {
	reqBody := chatRequest{
		Model:       model,
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

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return tryFallback(system, prompt, model, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("AI API returned %d: %s", resp.StatusCode, string(respBody))
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			return tryFallback(system, prompt, model, err)
		}
		return "", err
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

// tryFallback retries once with the configured fallback model when the primary
// model failed transiently (timeout, rate limit, server error). The model
// guard prevents an endless fallback loop if the fallback model also fails.
func tryFallback(system, prompt, model string, err error) (string, error) {
	if aiFallbackModel == "" || model == aiFallbackModel {
		return "", err
	}
	slog.Warn("AI primary model failed, falling back", slog.String("from", model), slog.String("to", aiFallbackModel), slog.Any("err", err))
	return callAIWithModel(system, prompt, aiFallbackModel)
}
