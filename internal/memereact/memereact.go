package memereact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

const (
	minReactions  = 2
	maxLogEntries = 50
)

type Config struct {
	AIAPIKey    string
	AIModel     string
	AIBaseURL   string
	GiphyAPIKey string
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

func (r *Reactor) OnReactionAdd(event *events.GuildMessageReactionAdd) {
	if event.UserID == event.Client().ApplicationID {
		return
	}
	if event.Member.User.Bot {
		return
	}

	msgID := event.MessageID
	r.mu.Lock()
	if last, ok := r.coolDown[msgID]; ok && time.Since(last) < 24*time.Hour {
		r.mu.Unlock()
		return
	}
	r.coolDown[msgID] = time.Now()
	r.mu.Unlock()

	msg, err := event.Client().Rest.GetMessage(event.ChannelID, msgID)
	if err != nil {
		return
	}

	reactionCount := 0
	for _, rxn := range msg.Reactions {
		if rxn.Count >= minReactions {
			reactionCount += rxn.Count
		}
	}
	if reactionCount < minReactions {
		return
	}

	content := msg.Content
	if content == "" {
		return
	}

	go r.process(event, content)
}

func (r *Reactor) process(event *events.GuildMessageReactionAdd, content string) {
	query, ok := r.aiGate(content)
	if !ok {
		r.log.add(content, "NO", "")
		return
	}

	gifURL, err := r.searchGiphy(query)
	if err != nil {
		slog.Warn("giphy search failed", slog.Any("err", err))
		return
	}

	r.log.add(content, "YES", query)

	embed := discord.Embed{
		Image: &discord.EmbedResource{URL: gifURL},
		Color: 0x9B59B6,
	}
	_, _ = event.Client().Rest.CreateMessage(event.ChannelID, discord.NewMessageCreate().WithEmbeds(embed))
}

func (r *Reactor) aiGate(content string) (string, bool) {
	reqBody := map[string]interface{}{
		"model":       r.cfg.AIModel,
		"temperature": 0.3,
		"messages": []map[string]string{
			{"role": "system", "content": "You analyze messages and decide if they'd be fun to react to with a GIF/meme. If YES, provide 1-3 short English Giphy search terms. Reply with just the terms or NO."},
			{"role": "user", "content": content},
		},
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest(http.MethodPost, r.cfg.AIBaseURL+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+r.cfg.AIAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("ai gate failed", slog.Any("err", err))
		return "", false
	}
	defer resp.Body.Close()

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
