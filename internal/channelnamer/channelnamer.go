package channelnamer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

type Config struct {
	ChannelIDs   []snowflake.ID
	APIKey       string
	Model        string
	BaseURL      string
	LogChannelID snowflake.ID
}

type Namer struct {
	config     Config
	httpClient *http.Client
	recent     []string
	mu         sync.Mutex
}

func New(cfg Config) *Namer {
	if len(cfg.ChannelIDs) == 0 || cfg.APIKey == "" {
		return nil
	}
	if cfg.Model == "" {
		cfg.Model = "big-pickle"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://opencode.ai/zen/v1"
	}
	return &Namer{
		config:     cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (n *Namer) Start(client *bot.Client) chan struct{} {
	stop := make(chan struct{})
	go func() {
		for {
			next := nextSchedule()
			t := time.NewTimer(next)
			select {
			case <-stop:
				t.Stop()
				return
			case <-t.C:
			}
			n.renameAll(client)
		}
	}()
	return stop
}

func nextSchedule() time.Duration {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
	if next.Before(now) || next.Equal(now) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now)
}

func (n *Namer) RenameAll(client *bot.Client) {
	n.renameAll(client)
}

func (n *Namer) renameAll(client *bot.Client) {
	names, err := n.generateNames(n.recent)
	if err != nil {
		slog.Warn("failed to generate channel names", slog.Any("err", err))
		return
	}

	n.mu.Lock()
	n.recent = names
	n.mu.Unlock()

	shuffled := make([]snowflake.ID, len(n.config.ChannelIDs))
	copy(shuffled, n.config.ChannelIDs)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	for i, channelID := range shuffled {
		if i >= len(names) {
			break
		}
		newName := names[i]

		channel, err := client.Rest.GetChannel(channelID)
		if err != nil {
			slog.Warn("failed to get channel for rename", slog.String("channel_id", channelID.String()), slog.Any("err", err))
			continue
		}
		gvc, ok := channel.(discord.GuildVoiceChannel)
		if !ok {
			slog.Warn("channel is not a voice channel", slog.String("channel_id", channelID.String()))
			continue
		}
		oldName := gvc.Name()
		if oldName == newName {
			continue
		}

		if _, err := client.Rest.UpdateChannel(channelID, discord.GuildVoiceChannelUpdate{Name: &newName}); err != nil {
			slog.Warn("failed to rename channel", slog.String("channel_id", channelID.String()), slog.Any("err", err))
			continue
		}
		slog.Info("renamed voice channel", slog.String("channel_id", channelID.String()), slog.String("from", oldName), slog.String("to", newName))

		if n.config.LogChannelID != 0 {
			embed := renameEmbed(oldName, newName)
			if _, err := client.Rest.CreateMessage(n.config.LogChannelID, discord.NewMessageCreate().WithEmbeds(embed)); err != nil {
				slog.Warn("failed to post rename log", slog.Any("err", err))
			}
		}
	}
}

func renameEmbed(oldName, newName string) discord.Embed {
	return discord.Embed{
		Title:       "Voice-Channel umbenannt",
		Description: fmt.Sprintf("**#%s** → **#%s**", oldName, newName),
		Color:       0x3498DB,
	}
}

var examples = []string{
	"Haus des heißen Dampfes 🔥💦",
	"Zum lodernden Lama 🐴💨",
	"Hot Bowl Club 🔥🍜🎉",
	"Zum versoffenen Magier 🧙🍷",
	"Zur Glitzersaft-Elfe ✨🧃🧚",
	"Gay-Gulasch-Garage 🍲🏳️‍🌈",
	"Die spritzigen Spritzer 💧",
}

func buildPrompt(channelIDs []snowflake.ID, recent []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Erzeuge %d kreative, lustige deutsche Voice-Channel-Namen im Stil einer mittelalterlichen Fantasy-Taverne, jeweils mit 1-3 passenden Emojis.\n\n", len(channelIDs)))
	b.WriteString("Orientiere dich an diesen Beispielen:\n")
	for _, ex := range examples {
		b.WriteString("- " + ex + "\n")
	}
	if len(recent) > 0 {
		b.WriteString("\nVerwende NICHT diese bereits kürzlich genutzten Namen:\n")
		for _, r := range recent {
			b.WriteString("- " + r + "\n")
		}
	}
	b.WriteString(fmt.Sprintf("\nAntworte NUR mit %d Zeilen, eine pro Name. Kein Prefix, kein JSON, keine Nummerierung.", len(channelIDs)))
	return b.String()
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

func (n *Namer) generateNames(recent []string) ([]string, error) {
	prompt := buildPrompt(n.config.ChannelIDs, recent)
	reqBody := chatRequest{
		Model:       n.config.Model,
		Temperature: 0.9,
		Messages: []chatMessage{
			{Role: "system", Content: "Du bist ein kreativer Namensgenerator für Discord-Voice-Channel."},
			{Role: "user", Content: prompt},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, n.config.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+n.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AI API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var cr chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, err
	}
	if len(cr.Choices) == 0 {
		return nil, fmt.Errorf("no choices in AI response")
	}

	names, ok := parseNames(cr.Choices[0].Message.Content)
	if !ok {
		return nil, fmt.Errorf("AI returned too few names: %d", len(names))
	}
	return names, nil
}

func parseNames(response string) ([]string, bool) {
	var names []string
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		names = append(names, trimmed)
	}
	return names, len(names) >= 2
}
