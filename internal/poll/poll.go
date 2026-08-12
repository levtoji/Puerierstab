package poll

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

type Store struct {
	polls    map[string]*Poll
	filePath string
	mu       sync.RWMutex
}

func NewStore() *Store {
	s := &Store{
		polls:    make(map[string]*Poll),
		filePath: ".polls.json",
	}
	s.load()
	return s
}

func (s *Store) save() {
	tmpPath := s.filePath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		slog.Warn("failed to create poll save file", slog.Any("err", err))
		return
	}
	if err := json.NewEncoder(f).Encode(s.polls); err != nil {
		f.Close()
		slog.Warn("failed to encode polls", slog.Any("err", err))
		return
	}
	f.Close()
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		slog.Warn("failed to rename poll save file", slog.Any("err", err))
	}
}

func (s *Store) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read poll save file", slog.Any("err", err))
		}
		return
	}
	var polls map[string]*Poll
	if err := json.Unmarshal(data, &polls); err != nil {
		slog.Warn("failed to parse poll save file", slog.Any("err", err))
		return
	}
	now := time.Now()
	for id, p := range polls {
		if now.Sub(p.CreatedAt) > 24*time.Hour {
			continue
		}
		if p.Votes == nil {
			p.Votes = make(map[int]map[snowflake.ID]struct{})
		}
		s.polls[id] = p
	}
	slog.Info("loaded polls", slog.Int("count", len(s.polls)))
}

func (s *Store) Create(question string, options []string, creatorID snowflake.ID) *Poll {
	p := &Poll{
		ID:        randomID(),
		Question:  question,
		Options:   options,
		Votes:     make(map[int]map[snowflake.ID]struct{}),
		CreatorID: creatorID,
		CreatedAt: time.Now(),
	}
	s.mu.Lock()
	s.polls[p.ID] = p
	s.save()
	s.mu.Unlock()
	return p
}

func (s *Store) Get(id string) (*Poll, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.polls[id]
	return p, ok
}

func (s *Store) StartCleanup() chan struct{} {
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(1 * time.Hour):
			}
			s.cleanupExpired()
		}
	}()
	return stop
}

func (s *Store) cleanupExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, p := range s.polls {
		if p.IsExpired() {
			delete(s.polls, id)
		}
	}
}

func (s *Store) ToggleVote(pollID string, optionIdx int, userID snowflake.ID) bool {
	s.mu.RLock()
	p, ok := s.polls[pollID]
	s.mu.RUnlock()
	if !ok {
		return false
	}
	p.ToggleVote(optionIdx, userID)
	s.mu.Lock()
	s.save()
	s.mu.Unlock()
	return true
}

func randomID() string {
	return strconv.FormatInt(rand.Int63(), 36)
}

type Poll struct {
	ID        string
	Question  string
	Options   []string
	Votes     map[int]map[snowflake.ID]struct{}
	CreatorID snowflake.ID
	CreatedAt time.Time
	mu        sync.RWMutex
}

func (p *Poll) ToggleVote(optionIdx int, userID snowflake.ID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Votes[optionIdx] == nil {
		p.Votes[optionIdx] = make(map[snowflake.ID]struct{})
	}
	if _, ok := p.Votes[optionIdx][userID]; ok {
		delete(p.Votes[optionIdx], userID)
	} else {
		p.Votes[optionIdx][userID] = struct{}{}
	}
}

func (p *Poll) VoteCount(optionIdx int) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.Votes[optionIdx])
}

func (p *Poll) HasVoted(userID snowflake.ID, optionIdx int) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.Votes[optionIdx] == nil {
		return false
	}
	_, ok := p.Votes[optionIdx][userID]
	return ok
}

func (p *Poll) Components() []discord.LayoutComponent {
	rows := make([]discord.LayoutComponent, len(p.Options))
	for i, opt := range p.Options {
		customID := fmt.Sprintf("poll:%s:%d", p.ID, i)
		btn := discord.NewPrimaryButton(opt, customID)
		rows[i] = discord.ActionRowComponent{Components: []discord.InteractiveComponent{btn}}
	}
	return rows
}

func (p *Poll) IsExpired() bool {
	return time.Since(p.CreatedAt) > 24*time.Hour
}

func (p *Poll) Embed() discord.Embed {
	p.mu.RLock()
	defer p.mu.RUnlock()
	title := p.Question
	if p.IsExpired() {
		title += " (abgelaufen)"
	}
	fields := make([]discord.EmbedField, len(p.Options))
	for i, opt := range p.Options {
		count := len(p.Votes[i])
		fields[i] = discord.EmbedField{
			Name:  opt,
			Value: fmt.Sprintf("%d Stimmen", count),
		}
	}
	return discord.Embed{
		Title:  title,
		Color:  0x5865F2,
		Fields: fields,
	}
}

func parseOptions(input string) ([]string, error) {
	parts := strings.Split(input, ",")
	var options []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		options = append(options, trimmed)
	}
	if len(options) < 2 {
		return nil, fmt.Errorf("mindestens 2 Optionen erforderlich, %d angegeben", len(options))
	}
	if len(options) > 5 {
		return nil, fmt.Errorf("maximal 5 Optionen, %d angegeben", len(options))
	}
	return options, nil
}

func ParseCustomID(customID string) (pollID string, optionIdx int, ok bool) {
	if !strings.HasPrefix(customID, "poll:") {
		return "", 0, false
	}
	parts := strings.Split(customID, ":")
	if len(parts) != 3 {
		return "", 0, false
	}
	idx, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", 0, false
	}
	return parts[1], idx, true
}

func (s *Store) HandleCreate(event *events.ApplicationCommandInteractionCreate) {
	data := event.SlashCommandInteractionData()
	question := data.String("question")
	optionsRaw := data.String("options")

	options, err := parseOptions(optionsRaw)
	if err != nil {
		_ = event.CreateMessage(discord.NewMessageCreate().
			WithContent("Fehler: " + err.Error()).
			WithEphemeral(true))
		return
	}

	if err := event.DeferCreateMessage(false); err != nil {
		slog.Warn("failed to defer poll create", slog.Any("err", err))
		return
	}

	poll := s.Create(question, options, event.User().ID)

	msg, err := event.Client().Rest.UpdateInteractionResponse(
		event.ApplicationID(),
		event.Token(),
		discord.NewMessageUpdate().
			WithEmbeds(poll.Embed()).
			WithComponents(poll.Components()...),
	)
	if err != nil {
		slog.Warn("failed to create poll message", slog.Any("err", err))
		return
	}

	slog.Info("created poll", slog.String("id", poll.ID), slog.String("message_id", msg.ID.String()))
}

func (s *Store) HandleComponent(event *events.ComponentInteractionCreate) {
	if event.Data.Type() != discord.ComponentTypeButton {
		return
	}
	pollID, optionIdx, ok := ParseCustomID(event.ButtonInteractionData().CustomID())
	if !ok {
		return
	}

	poll, found := s.Get(pollID)
	if !found {
		return
	}

	if poll.IsExpired() {
		_ = event.UpdateMessage(discord.NewMessageUpdate().
			WithEmbeds(poll.Embed()).
			WithComponents(poll.Components()...))
		_ = event.CreateMessage(discord.NewMessageCreate().
			WithContent("Diese Umfrage ist abgelaufen.").
			WithEphemeral(true))
		return
	}

	userID := event.User().ID
	s.ToggleVote(pollID, optionIdx, userID)

	msg := discord.NewMessageUpdate().
		WithEmbeds(poll.Embed()).
		WithComponents(poll.Components()...)

	if err := event.UpdateMessage(msg); err != nil {
		slog.Warn("failed to update poll message", slog.Any("err", err))
	}
}
