package memereact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

const (
	minReactions     = 2
	maxLogEntries    = 50
	coolDownDuration = 30 * time.Minute
)

type Config struct {
	AIAPIKey        string
	AIModel         string
	AIFallbackModel string
	AIBaseURL       string
	GiphyAPIKey     string
}

type MemeEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Content   string    `json:"content"`
	Decision  string    `json:"decision"`
	Query     string    `json:"query"`
}

type MemeLog struct {
	entries []MemeEntry
	mu      sync.Mutex
}

func (l *MemeLog) add(content, decision, query string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(content) > 100 {
		content = content[:100]
	}
	l.entries = append(l.entries, MemeEntry{
		Timestamp: time.Now(),
		Content:   content,
		Decision:  decision,
		Query:     query,
	})
	if len(l.entries) > maxLogEntries {
		l.entries = l.entries[len(l.entries)-maxLogEntries:]
	}
}

func (l *MemeLog) Recent() []MemeEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]MemeEntry, len(l.entries))
	for i, e := range l.entries {
		result[i] = e
	}
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

type Reactor struct {
	cfg      Config
	coolDown map[snowflake.ID]time.Time
	log      *MemeLog
	mu       sync.Mutex
}

func New(cfg Config) *Reactor {
	if cfg.AIAPIKey == "" || cfg.GiphyAPIKey == "" {
		return nil
	}
	if cfg.AIModel == "" {
		cfg.AIModel = "big-pickle"
	}
	if cfg.AIBaseURL == "" {
		cfg.AIBaseURL = "https://opencode.ai/zen/v1"
	}
	return &Reactor{
		cfg:      cfg,
		coolDown: make(map[snowflake.ID]time.Time),
		log:      &MemeLog{},
	}
}

func (r *Reactor) MemeLog() *MemeLog { return r.log }

// tryBegin atomically checks the message cooldown, prunes expired entries
// and marks the message as in-progress. Returns true if processing may start.
func (r *Reactor) tryBegin(msgID snowflake.ID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if last, ok := r.coolDown[msgID]; ok {
		if time.Since(last) < coolDownDuration {
			return false
		}
		delete(r.coolDown, msgID)
	}
	r.coolDown[msgID] = time.Now()
	return true
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}

func reactionCount(msg *discord.Message) int {
	total := 0
	for _, rxn := range msg.Reactions {
		count := rxn.Count
		if rxn.Me {
			count--
		}
		if count > 0 {
			total += count
		}
	}
	return total
}

func (r *Reactor) ForceMeme(content string) (string, error) {
	query, ok := r.aiGate(content)
	if !ok {
		r.log.add(content, "NO", "")
		return "", fmt.Errorf("AI gate said NO")
	}
	gifURL, err := r.searchGiphy(query)
	if err != nil {
		return "", err
	}
	r.log.add(content, "YES", query)
	return gifURL, nil
}

func (r *Reactor) OnReactionAdd(event *events.GuildMessageReactionAdd) {
	if event.UserID == event.Client().ApplicationID {
		return
	}
	if event.Member.User.Bot {
		return
	}

	msgID := event.MessageID
	client := event.Client()

	msg, err := client.Rest.GetMessage(event.ChannelID, msgID)
	if err != nil {
		slog.Warn("meme gate: failed to fetch message", slog.Any("err", err))
		return
	}

	totalReactions := reactionCount(msg)
	if totalReactions < minReactions {
		return
	}

	content := msg.Content
	if content == "" {
		return
	}

	if !r.tryBegin(msgID) {
		return
	}

	context := content
	prev, err := client.Rest.GetMessages(event.ChannelID, 0, msgID, 0, 3)
	if err == nil && len(prev) > 0 {
		var parts []string
		for i := len(prev) - 1; i >= 0; i-- {
			if prev[i].Content != "" {
				parts = append(parts, prev[i].Content)
			}
		}
		parts = append(parts, content)
		if len(parts) > 1 {
			context = strings.Join(parts, "\n---\n")
		}
	}

	go r.process(event, context)
}

func (r *Reactor) process(event *events.GuildMessageReactionAdd, content string) {
	query, ok := r.aiGate(content)
	if !ok {
		r.log.add(content, "NO", "")
		slog.Info("meme gate: NO", slog.String("content", truncate(content, 100)))
		return
	}

	gifURL, err := r.searchGiphy(query)
	if err != nil {
		slog.Warn("giphy search failed", slog.String("query", query), slog.Any("err", err))
		return
	}

	r.log.add(content, "YES", query)
	slog.Info("meme gate: YES", slog.String("content", truncate(content, 100)), slog.String("query", query))

	embed := discord.Embed{
		Image: &discord.EmbedResource{URL: gifURL},
		Color: 0x9B59B6,
	}
	_, _ = event.Client().Rest.CreateMessage(event.ChannelID, discord.NewMessageCreate().WithEmbeds(embed))
}

func (r *Reactor) aiGate(content string) (string, bool) {
	return r.aiGateWithModel(content, r.cfg.AIModel)
}

func (r *Reactor) aiGateWithModel(content, model string) (string, bool) {
	reqBody := map[string]interface{}{
		"model":       model,
		"temperature": 0.3,
		"messages": []map[string]string{
			{"role": "system", "content": "You analyze conversation context and decide if the last message deserves a GIF/meme reaction. Consider sarcasm, callbacks, and jokes. If YES, provide 1-3 short English Giphy search terms. Reply with just the terms or NO."},
			{"role": "user", "content": content},
		},
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest(http.MethodPost, r.cfg.AIBaseURL+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+r.cfg.AIAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return r.aiGateWithFallback(content, model, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err := fmt.Errorf("AI API returned %d", resp.StatusCode)
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			return r.aiGateWithFallback(content, model, err)
		}
		slog.Warn("ai gate failed", slog.Any("err", err))
		return "", false
	}

	var cr struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil || len(cr.Choices) == 0 {
		return "", false
	}

	result := cr.Choices[0].Message.Content
	if result == "" || result == "NO" || result == "No" || result == "no" {
		return "", false
	}
	return result, true
}

// aiGateWithFallback retries once with the configured fallback model when the
// primary model failed transiently (timeout, rate limit, server error). The
// model guard prevents an endless fallback loop.
func (r *Reactor) aiGateWithFallback(content, model string, err error) (string, bool) {
	if r.cfg.AIFallbackModel == "" || model == r.cfg.AIFallbackModel {
		slog.Warn("ai gate failed", slog.Any("err", err))
		return "", false
	}
	slog.Warn("AI primary model failed, falling back", slog.String("from", model), slog.String("to", r.cfg.AIFallbackModel), slog.Any("err", err))
	return r.aiGateWithModel(content, r.cfg.AIFallbackModel)
}

type giphyResponse struct {
	Data []struct {
		Images struct {
			Original struct {
				URL string `json:"url"`
			} `json:"original"`
		} `json:"images"`
	} `json:"data"`
}

func (r *Reactor) searchGiphy(query string) (string, error) {
	u, _ := url.Parse("https://api.giphy.com/v1/gifs/search")
	u.RawQuery = url.Values{
		"api_key": {r.cfg.GiphyAPIKey},
		"q":       {query},
		"limit":   {"3"},
		"rating":  {"pg-13"},
		"lang":    {"en"},
	}.Encode()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(u.String())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("giphy %d: %s", resp.StatusCode, string(body))
	}

	var gr giphyResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return "", err
	}
	if len(gr.Data) == 0 {
		return "", fmt.Errorf("no gifs found for %q", query)
	}

	return gr.Data[0].Images.Original.URL, nil
}
