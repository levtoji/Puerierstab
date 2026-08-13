package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/levtoji/Puerierstab/internal/chatlog"
	"github.com/levtoji/Puerierstab/internal/reactions"
)

const (
	maxHistoryChars = 1500
	window          = 90 * 24 * time.Hour
)

type Config struct {
	APIKey        string
	Model         string
	FallbackModel string
	BaseURL       string
	Dir           string
}

type Profile struct {
	Text      string    `json:"text"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Store struct {
	Profiles map[snowflake.ID]Profile `json:"profiles"`
	filePath string
	mu       sync.RWMutex
}

type Profiler struct {
	cfg        Config
	chat       *chatlog.Logger
	reactions  *reactions.Logger
	store      *Store
	httpClient *http.Client
}

func New(cfg Config, chat *chatlog.Logger, rx *reactions.Logger) *Profiler {
	if cfg.APIKey == "" {
		return nil
	}
	if cfg.Model == "" {
		cfg.Model = "big-pickle"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://opencode.ai/zen/v1"
	}
	store := &Store{Profiles: make(map[snowflake.ID]Profile), filePath: filepath.Join(cfg.Dir, ".profiles.json")}
	store.load()
	return &Profiler{
		cfg:        cfg,
		chat:       chat,
		reactions:  rx,
		store:      store,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *Profiler) Get(userID snowflake.ID) (Profile, bool) {
	p.store.mu.RLock()
	defer p.store.mu.RUnlock()
	prof, ok := p.store.Profiles[userID]
	return prof, ok
}

func (p *Profiler) All() map[snowflake.ID]Profile {
	return p.store.All()
}

func (s *Store) All() map[snowflake.ID]Profile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[snowflake.ID]Profile, len(s.Profiles))
	for id, prof := range s.Profiles {
		out[id] = prof
	}
	return out
}

func (p *Profiler) Start() chan struct{} {
	stop := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("profile pipeline panic", slog.Any("panic", r))
			}
		}()
		for {
			if !sleepUntil(stop, nextSchedule()) {
				return
			}
			p.RunOnce()
		}
	}()
	return stop
}

func sleepUntil(stop chan struct{}, d time.Duration) bool {
	t := time.NewTimer(d)
	select {
	case <-stop:
		t.Stop()
		return false
	case <-t.C:
		return true
	}
}

func nextSchedule() time.Duration {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
	if next.Before(now) || next.Equal(now) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now)
}

func (p *Profiler) RunOnce() int {
	if p.chat == nil {
		return 0
	}
	updated := 0
	for _, userID := range p.chat.AllUsers() {
		msgs := p.chat.GetMessages(userID, window)
		if len(msgs) == 0 {
			continue
		}
		history := trimHistory(msgs)
		var given, received map[string]int
		if p.reactions != nil {
			given, received = p.reactions.Stats(userID, window)
		}

		name := "User"
		text, err := p.generateProfile(userID, name, history, given, received)
		if err != nil {
			slog.Warn("profile generation failed", slog.String("user_id", userID.String()), slog.Any("err", err))
			continue
		}
		if text == "" {
			continue
		}
		p.store.mu.Lock()
		p.store.Profiles[userID] = Profile{Text: text, UpdatedAt: time.Now()}
		p.store.save()
		p.store.mu.Unlock()
		updated++
		slog.Info("profile updated", slog.String("user_id", userID.String()))
	}
	return updated
}

func trimHistory(msgs []string) string {
	var trimmed []string
	total := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		if total+len(msgs[i]) > maxHistoryChars {
			break
		}
		trimmed = append([]string{msgs[i]}, trimmed...)
		total += len(msgs[i])
	}
	return strings.Join(trimmed, "\n")
}

func formatEmojiTop(counts map[string]int, limit int) string {
	if len(counts) == 0 {
		return ""
	}
	type pair struct {
		emoji string
		count int
	}
	pairs := make([]pair, 0, len(counts))
	for e, c := range counts {
		pairs = append(pairs, pair{e, c})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].emoji < pairs[j].emoji
	})
	var parts []string
	for i, p := range pairs {
		if i >= limit {
			break
		}
		parts = append(parts, fmt.Sprintf("%s (%d)", p.emoji, p.count))
	}
	return strings.Join(parts, ", ")
}

func buildProfilePrompt(name, history string, given, received map[string]int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Nachrichten von @%s (letzte 3 Monate):\n%s\n", name, history))
	if g := formatEmojiTop(given, 5); g != "" {
		b.WriteString(fmt.Sprintf("\nTop-Reaktionen, die @%s vergibt: %s\n", name, g))
	}
	if r := formatEmojiTop(received, 5); r != "" {
		b.WriteString(fmt.Sprintf("\nTop-Reaktionen auf @%s's Nachrichten: %s\n", name, r))
	}
	b.WriteString("\nSchreibe ein neutrales, sachliches Persönlichkeitsprofil von 2-3 Sätzen. Trocken, wertfrei und präzise. Deutsch.")
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

func (p *Profiler) generateProfile(userID snowflake.ID, name, history string, given, received map[string]int) (string, error) {
	return p.generateProfileWithModel(userID, name, history, given, received, p.cfg.Model)
}

func (p *Profiler) generateProfileWithModel(userID snowflake.ID, name, history string, given, received map[string]int, model string) (string, error) {
	prompt := buildProfilePrompt(name, history, given, received)
	reqBody := chatRequest{
		Model:       model,
		Temperature: 0.7,
		Messages: []chatMessage{
			{Role: "system", Content: "Du erstellst ein neutrales, sachliches Persönlichkeitsprofil eines Discord-Users basierend auf seinen Nachrichten und Reaktionen. Deutsch."},
			{Role: "user", Content: prompt},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, p.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return p.generateProfileWithFallback(userID, name, history, given, received, model, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("AI API returned %d: %s", resp.StatusCode, string(respBody))
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			return p.generateProfileWithFallback(userID, name, history, given, received, model, err)
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

	return strings.TrimSpace(cr.Choices[0].Message.Content), nil
}

// generateProfileWithFallback retries once with the configured fallback model
// when the primary model failed transiently (timeout, rate limit, server
// error). The model guard prevents an endless fallback loop.
func (p *Profiler) generateProfileWithFallback(userID snowflake.ID, name, history string, given, received map[string]int, model string, err error) (string, error) {
	if p.cfg.FallbackModel == "" || model == p.cfg.FallbackModel {
		return "", err
	}
	slog.Warn("AI primary model failed, falling back", slog.String("from", model), slog.String("to", p.cfg.FallbackModel), slog.Any("err", err))
	return p.generateProfileWithModel(userID, name, history, given, received, p.cfg.FallbackModel)
}

func (s *Store) save() {
	tmpPath := s.filePath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		slog.Warn("failed to create profiles save file", slog.Any("err", err))
		return
	}
	if err := json.NewEncoder(f).Encode(s); err != nil {
		f.Close()
		slog.Warn("failed to encode profiles", slog.Any("err", err))
		return
	}
	f.Close()
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		slog.Warn("failed to rename profiles save file", slog.Any("err", err))
	}
}

func (s *Store) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read profiles file", slog.Any("err", err))
		}
		return
	}
	if err := json.Unmarshal(data, s); err != nil {
		slog.Warn("failed to parse profiles file", slog.Any("err", err))
		s.Profiles = make(map[snowflake.ID]Profile)
		return
	}
	slog.Info("loaded profiles", slog.Int("users", len(s.Profiles)))
}
