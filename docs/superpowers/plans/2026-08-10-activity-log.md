# Activity-Log Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build activity logging for member join/leave/nickname-change/role-change/voice-channel-change into a private Discord channel, while refactoring the role panel logic into feature packages.

**Architecture:** Feature packages (`internal/config`, `internal/rolepanel`, `internal/activitylog`), main.go as a thin wiring layer. Config loading moved to `internal/config` to avoid import cycle. Activity-log handlers are pure event-to-embed-to-post, with testable diff and embed-builder functions.

**Tech Stack:** Go 1.24.13, disgo v0.19.6, snowflake v2

**Deviation from spec:** The design doc says `config.go` stays in `main`, but `internal/rolepanel` needs `RoleCategory`/`RoleButton`/`Config` types and `BuildCategoryMessages`/`MemberHasRole` functions. Keeping `config.go` in `main` would create a `main → rolepanel → main` import cycle. Therefore `config.go` moves to `internal/config` as well. All other spec decisions stand.

---

### Task 1: Move config.go → internal/config

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Delete: `config.go`
- Delete: `config_test.go`

- [ ] **Step 1: Create directories**

```bash
mkdir -p internal/config
```

- [ ] **Step 2: Create internal/config/config.go**

`internal/config/config.go` — copy of current `config.go`, package renamed to `config`, types and functions that are used cross-package exported (`Config`, `RoleCategory`, `RoleButton`, `LoadConfigFromEnv`, `BuildCategoryMessages`, `MemberHasRole`):

```go
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

const (
	roleConfigEnv     = "ROLE_CATEGORIES_JSON"
	roleChannelEnv    = "ROLE_CHANNEL_ID"
	maxButtonsPerRow  = 5
	maxRowsPerMessage = 5
)

type Config struct {
	Token         string
	RoleChannelID snowflake.ID
	Categories    []RoleCategory
}

type RoleCategory struct {
	Name        string
	Description string
	Emoji       string
	Roles       []RoleButton
}

type RoleButton struct {
	RoleID      snowflake.ID
	Label       string
	Description string
	CustomID    string
	Style       string
}

func LoadConfigFromEnv() (Config, error) {
	token := strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN"))
	if token == "" {
		return Config{}, errors.New("missing DISCORD_BOT_TOKEN")
	}

	roleChannelID, err := parseSnowflakeEnv(roleChannelEnv)
	if err != nil {
		return Config{}, err
	}

	categories, err := loadRoleCategories()
	if err != nil {
		return Config{}, err
	}

	return Config{
		Token:         token,
		RoleChannelID: roleChannelID,
		Categories:    categories,
	}, nil
}

func loadRoleCategories() ([]RoleCategory, error) {
	raw := strings.TrimSpace(os.Getenv(roleConfigEnv))
	if raw == "" {
		return nil, fmt.Errorf("missing %s", roleConfigEnv)
	}

	var categories []roleCategoryJSON
	if err := json.Unmarshal([]byte(raw), &categories); err != nil {
		return nil, fmt.Errorf("parse %s: %w", roleConfigEnv, err)
	}
	return normalizeCategories(categories)
}

func normalizeCategories(rawCategories []roleCategoryJSON) ([]RoleCategory, error) {
	if len(rawCategories) == 0 {
		return nil, errors.New("at least one role category is required")
	}

	categories := make([]RoleCategory, 0, len(rawCategories))
	seenCustomIDs := make(map[string]struct{})

	for categoryIndex, rawCategory := range rawCategories {
		category := RoleCategory{
			Name:        strings.TrimSpace(rawCategory.Name),
			Description: strings.TrimSpace(rawCategory.Description),
			Emoji:       strings.TrimSpace(rawCategory.Emoji),
		}
		if category.Name == "" {
			return nil, fmt.Errorf("category %d: name is required", categoryIndex)
		}
		if len(rawCategory.Roles) == 0 {
			return nil, fmt.Errorf("category %q: at least one role is required", category.Name)
		}

		category.Roles = make([]RoleButton, 0, len(rawCategory.Roles))
		for roleIndex, rawRole := range rawCategory.Roles {
			label := strings.TrimSpace(rawRole.Label)
			if label == "" {
				return nil, fmt.Errorf("category %q role %d: label is required", category.Name, roleIndex)
			}

			customID := strings.TrimSpace(rawRole.CustomID)
			if customID == "" {
				customID = fmt.Sprintf("role_toggle:%s", rawRole.RoleID.String())
			}
			if _, exists := seenCustomIDs[customID]; exists {
				return nil, fmt.Errorf("duplicate custom_id %q", customID)
			}
			seenCustomIDs[customID] = struct{}{}

			category.Roles = append(category.Roles, RoleButton{
				RoleID:      snowflake.ID(rawRole.RoleID),
				Label:       label,
				Description: strings.TrimSpace(rawRole.Description),
				CustomID:    customID,
				Style:       strings.TrimSpace(rawRole.Style),
			})
		}

		categories = append(categories, category)
	}

	return categories, nil
}

func BuildCategoryMessages(category RoleCategory) []discord.MessageCreate {
	const maxButtonsPerMessage = maxButtonsPerRow * maxRowsPerMessage

	if len(category.Roles) == 0 {
		return nil
	}

	messages := make([]discord.MessageCreate, 0, (len(category.Roles)+maxButtonsPerMessage-1)/maxButtonsPerMessage)
	for start := 0; start < len(category.Roles); start += maxButtonsPerMessage {
		end := start + maxButtonsPerMessage
		if end > len(category.Roles) {
			end = len(category.Roles)
		}
		chunk := category.Roles[start:end]

		embed := discord.Embed{
			Title:       category.Emoji + " " + category.Name,
			Description: category.Description,
			Color:       0x5865F2,
			Fields:      []discord.EmbedField{},
		}

		for _, role := range chunk {
			fieldValue := role.Description
			if fieldValue == "" {
				fieldValue = "Keine Beschreibung"
			}
			embed.Fields = append(embed.Fields, discord.EmbedField{
				Name:  role.Label,
				Value: fieldValue,
			})
		}

		message := discord.NewMessageCreate().
			WithEmbeds(embed).
			WithAllowedMentions(&discord.AllowedMentions{
				Parse: []discord.AllowedMentionType{},
				Roles: []snowflake.ID{},
				Users: []snowflake.ID{},
			})

		for rowStart := 0; rowStart < len(chunk); rowStart += maxButtonsPerRow {
			rowEnd := rowStart + maxButtonsPerRow
			if rowEnd > len(chunk) {
				rowEnd = len(chunk)
			}

			buttons := make([]discord.InteractiveComponent, 0, rowEnd-rowStart)
			for _, role := range chunk[rowStart:rowEnd] {
				buttons = append(buttons, role.component())
			}
			message = message.AddActionRow(buttons...)
		}

		messages = append(messages, message)
	}

	return messages
}

func (r RoleButton) component() discord.ButtonComponent {
	switch strings.ToLower(r.Style) {
	case "success":
		return discord.NewSuccessButton(r.Label, r.CustomID)
	case "danger":
		return discord.NewDangerButton(r.Label, r.CustomID)
	case "secondary":
		return discord.NewSecondaryButton(r.Label, r.CustomID)
	default:
		return discord.NewPrimaryButton(r.Label, r.CustomID)
	}
}

func MemberHasRole(roleIDs []snowflake.ID, roleID snowflake.ID) bool {
	for _, existingRoleID := range roleIDs {
		if existingRoleID == roleID {
			return true
		}
	}
	return false
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			return value
		}
	}
	return ""
}

func parseSnowflakeEnv(key string) (snowflake.ID, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0, fmt.Errorf("%s is required", key)
	}
	return parseSnowflake(value)
}

func parseSnowflake(raw string) (snowflake.ID, error) {
	id, err := snowflake.Parse(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid snowflake %q: %w", raw, err)
	}
	if id == 0 {
		return 0, errors.New("snowflake must not be zero")
	}
	return id, nil
}

type roleCategoryJSON struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Emoji       string           `json:"emoji"`
	Roles       []roleButtonJSON `json:"roles"`
}

type roleButtonJSON struct {
	RoleID      jsonSnowflake `json:"role_id"`
	Label       string        `json:"label"`
	Description string        `json:"description"`
	CustomID    string        `json:"custom_id"`
	Style       string        `json:"style"`
}

type jsonSnowflake snowflake.ID

func (id *jsonSnowflake) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		var number json.Number
		if err := json.Unmarshal(data, &number); err != nil {
			return fmt.Errorf("snowflake must be a string or number: %w", err)
		}
		raw = number.String()
	}

	parsed, err := parseSnowflake(raw)
	if err != nil {
		return err
	}
	*id = jsonSnowflake(parsed)
	return nil
}

func (id jsonSnowflake) String() string {
	return snowflake.ID(id).String()
}

func (id jsonSnowflake) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(id.String())), nil
}
```

- [ ] **Step 3: Create internal/config/config_test.go**

`internal/config/config_test.go` — copy of current `config_test.go`, package `config` (internal white-box, can test unexported), update type references from unexported to exported where appropriate:

```go
package config

import (
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func TestLoadConfigFromEnvWithJSONConfig(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "token")
	t.Setenv(roleChannelEnv, "123456789012345678")
	t.Setenv(roleConfigEnv, `[{"name":"Filme","description":"Filmrollen","emoji":"🎬","roles":[{"role_id":"987654321098765432","label":"Film","description":"Ping für Filme","custom_id":"film_role_toggle","style":"success"}]}]`)

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() error = %v", err)
	}

	if cfg.Token != "token" {
		t.Fatalf("expected token to be loaded from DISCORD_BOT_TOKEN, got %q", cfg.Token)
	}
	if cfg.RoleChannelID != snowflake.MustParse("123456789012345678") {
		t.Fatalf("unexpected role channel id: %s", cfg.RoleChannelID)
	}
	if len(cfg.Categories) != 1 || len(cfg.Categories[0].Roles) != 1 {
		t.Fatalf("unexpected category layout: %+v", cfg.Categories)
	}

	category := cfg.Categories[0]
	if category.Emoji != "🎬" {
		t.Fatalf("expected emoji 🎬, got %q", category.Emoji)
	}

	role := category.Roles[0]
	if role.CustomID != "film_role_toggle" {
		t.Fatalf("unexpected custom id: %q", role.CustomID)
	}
	if role.Style != "success" {
		t.Fatalf("unexpected style: %q", role.Style)
	}
}

func TestLoadConfigFromEnvMissingRoleCategoriesJSON(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "token")
	t.Setenv(roleChannelEnv, "123456789012345678")
	t.Setenv(roleConfigEnv, "")

	_, err := LoadConfigFromEnv()
	if err == nil {
		t.Fatalf("expected error when ROLE_CATEGORIES_JSON is missing, got nil")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected error message to contain 'missing', got %q", err.Error())
	}
}

func TestLoadConfigFromEnvMissingToken(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "")
	t.Setenv(roleChannelEnv, "123456789012345678")
	t.Setenv(roleConfigEnv, `[{"name":"Test","emoji":"🎮","roles":[]}]`)

	_, err := LoadConfigFromEnv()
	if err == nil {
		t.Fatalf("expected error when DISCORD_BOT_TOKEN is missing, got nil")
	}
	if !strings.Contains(err.Error(), "DISCORD_BOT_TOKEN") {
		t.Fatalf("expected error message to mention DISCORD_BOT_TOKEN, got %q", err.Error())
	}
}

func TestBuildCategoryMessagesChunksButtonsIntoMultipleMessages(t *testing.T) {
	roles := make([]RoleButton, 0, 26)
	for i := 0; i < 26; i++ {
		roles = append(roles, RoleButton{
			RoleID:      snowflake.ID(1000 + i),
			Label:       "Rolle " + string(rune('A'+(i%26))),
			Description: "Desc " + string(rune('a'+(i%26))),
			CustomID:    "custom-" + string(rune('a'+(i%26))),
		})
	}

	messages := BuildCategoryMessages(RoleCategory{
		Name:        "Games",
		Description: "Selbstbedienungsrollen",
		Emoji:       "🎮",
		Roles:       roles,
	})

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages for 26 buttons, got %d", len(messages))
	}

	if len(messages[0].Components) != maxRowsPerMessage {
		t.Fatalf("expected %d action rows in first message, got %d", maxRowsPerMessage, len(messages[0].Components))
	}
	if len(messages[0].Embeds) == 0 {
		t.Fatalf("expected embed in first message, got none")
	}
	embed := messages[0].Embeds[0]
	if embed.Title != "🎮 Games" {
		t.Fatalf("expected title '🎮 Games', got %q", embed.Title)
	}

	if len(messages[1].Components) != 1 {
		t.Fatalf("expected 1 action row in second message, got %d", len(messages[1].Components))
	}
}

func TestRoleButtonComponentUsesRequestedStyle(t *testing.T) {
	tests := []struct {
		style        string
		expectedEnum discord.ButtonStyle
	}{
		{"primary", discord.ButtonStylePrimary},
		{"danger", discord.ButtonStyleDanger},
		{"success", discord.ButtonStyleSuccess},
		{"secondary", discord.ButtonStyleSecondary},
		{"unknown", discord.ButtonStylePrimary},
	}

	for _, tt := range tests {
		t.Run(tt.style, func(t *testing.T) {
			button := RoleButton{
				Label:    "Test",
				CustomID: "test",
				Style:    tt.style,
			}.component()
			if button.Style != tt.expectedEnum {
				t.Fatalf("expected style %v, got %v", tt.expectedEnum, button.Style)
			}
		})
	}
}

func TestMemberHasRole(t *testing.T) {
	roleID1 := snowflake.MustParse("111111111111111111")
	roleID2 := snowflake.MustParse("222222222222222222")
	roleID3 := snowflake.MustParse("333333333333333333")

	memberRoles := []snowflake.ID{roleID1, roleID3}

	tests := []struct {
		name     string
		roleID   snowflake.ID
		expected bool
	}{
		{"has role 1", roleID1, true},
		{"has role 3", roleID3, true},
		{"missing role 2", roleID2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MemberHasRole(memberRoles, tt.roleID)
			if result != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestNormalizeCategoriesValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   []roleCategoryJSON
		wantErr bool
		errMsg  string
	}{
		{
			"empty categories",
			[]roleCategoryJSON{},
			true,
			"at least one role category is required",
		},
		{
			"category without name",
			[]roleCategoryJSON{
				{Description: "Test", Roles: []roleButtonJSON{{Label: "R1", RoleID: jsonSnowflake(1)}}},
			},
			true,
			"name is required",
		},
		{
			"category without roles",
			[]roleCategoryJSON{
				{Name: "Gaming", Description: "Test", Roles: []roleButtonJSON{}},
			},
			true,
			"at least one role is required",
		},
		{
			"role without label",
			[]roleCategoryJSON{
				{Name: "Gaming", Roles: []roleButtonJSON{{RoleID: jsonSnowflake(1)}}},
			},
			true,
			"label is required",
		},
		{
			"duplicate custom_id",
			[]roleCategoryJSON{
				{
					Name: "Gaming",
					Roles: []roleButtonJSON{
						{Label: "R1", RoleID: jsonSnowflake(1), CustomID: "same_id"},
						{Label: "R2", RoleID: jsonSnowflake(2), CustomID: "same_id"},
					},
				},
			},
			true,
			"duplicate custom_id",
		},
		{
			"valid single role",
			[]roleCategoryJSON{
				{Name: "Gaming", Emoji: "🎮", Roles: []roleButtonJSON{{Label: "R1", RoleID: jsonSnowflake(1)}}},
			},
			false,
			"",
		},
		{
			"valid multiple categories",
			[]roleCategoryJSON{
				{Name: "Cat1", Emoji: "🎮", Roles: []roleButtonJSON{{Label: "R1", RoleID: jsonSnowflake(1)}}},
				{Name: "Cat2", Emoji: "🎬", Roles: []roleButtonJSON{{Label: "R2", RoleID: jsonSnowflake(2)}}},
			},
			false,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := normalizeCategories(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result == nil {
					t.Fatalf("expected categories, got nil")
				}
			}
		})
	}
}

func TestNormalizeCategoriesAutoGenerateCustomID(t *testing.T) {
	input := []roleCategoryJSON{
		{
			Name:  "Gaming",
			Emoji: "🎮",
			Roles: []roleButtonJSON{
				{Label: "R1", RoleID: jsonSnowflake(111)},
				{Label: "R2", RoleID: jsonSnowflake(222), CustomID: "custom_r2"},
			},
		},
	}

	result, err := normalizeCategories(input)
	if err != nil {
		t.Fatalf("normalizeCategories() error = %v", err)
	}

	roles := result[0].Roles
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roles))
	}

	if !strings.Contains(roles[0].CustomID, "111") {
		t.Fatalf("expected auto-generated custom_id to contain role ID 111, got %q", roles[0].CustomID)
	}

	if roles[1].CustomID != "custom_r2" {
		t.Fatalf("expected custom_r2, got %q", roles[1].CustomID)
	}
}
```

- [ ] **Step 4: Delete old files**

```bash
rm config.go config_test.go
```

- [ ] **Step 5: Run config tests to verify the move**

```bash
go test ./internal/config/ -v
```

Expected: all tests pass (11 tests).

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: move config to internal/config"
```

---

### Task 2: Create internal/rolepanel — panel + index helpers

**Files:**
- Create: `internal/rolepanel/rolepanel.go`
- Create: `internal/rolepanel/interaction.go`

- [ ] **Step 1: Create directory**

```bash
mkdir -p internal/rolepanel
```

- [ ] **Step 2: Create internal/rolepanel/rolepanel.go**

Moved from `main.go`: `roleBot` struct, `NewRoleBot`, panel publishing, message indexing, and all helpers (`customIDSet`, `collectButtonCustomIDs`, `messageCustomIDs`, `messageCreateCustomIDs`, `indexMessagesByCustomIDs`, `isManagedRolePanelMessage`, `messageUpdateFromCreate`, `totalRoleCount`):

```go
package rolepanel

import (
	"sort"
	"strings"
	"sync"

	"log/slog"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/levtoji/Puerierstab/internal/config"
)

type RoleBot struct {
	cfg            config.Config
	roleByCustomID map[string]config.RoleButton
	syncOnce       sync.Once
}

func NewRoleBot(cfg config.Config) *RoleBot {
	roleByCustomID := make(map[string]config.RoleButton, totalRoleCount(cfg.Categories))
	for _, category := range cfg.Categories {
		for _, role := range category.Roles {
			roleByCustomID[role.CustomID] = role
		}
	}

	return &RoleBot{
		cfg:            cfg,
		roleByCustomID: roleByCustomID,
	}
}

func (b *RoleBot) publishRolePanels(client *bot.Client) error {
	existingMessages, err := b.getRoleChannelMessages(client)
	if err != nil {
		return err
	}

	existingByCustomIDs := indexMessagesByCustomIDs(existingMessages)

	for _, category := range b.cfg.Categories {
		for messageIdx, messageCreate := range config.BuildCategoryMessages(category) {
			wantCustomIDs := messageCreateCustomIDs(messageCreate)

			messageID, ok := existingByCustomIDs[wantCustomIDs.key()]
			if ok {
				if _, err := client.Rest.UpdateMessage(b.cfg.RoleChannelID, messageID, messageUpdateFromCreate(messageCreate)); err != nil {
					slog.Warn("failed to update role panel, creating new", slog.String("category", category.Name), slog.Int("message_index", messageIdx), slog.Any("err", err))
				} else {
					delete(existingByCustomIDs, wantCustomIDs.key())
					continue
				}
			}

			created, err := client.Rest.CreateMessage(b.cfg.RoleChannelID, messageCreate)
			if err != nil {
				return err
			}
			slog.Info("created new role panel", slog.String("category", category.Name), slog.Int("message_index", messageIdx), slog.String("message_id", created.ID.String()))
		}
	}

	for _, messageID := range existingByCustomIDs {
		if err := client.Rest.DeleteMessage(b.cfg.RoleChannelID, messageID); err != nil {
			slog.Warn("failed to delete stale role panel", slog.String("message_id", messageID.String()), slog.Any("err", err))
		}
	}
	return nil
}

func (b *RoleBot) getRoleChannelMessages(client *bot.Client) ([]discord.Message, error) {
	var (
		messages []discord.Message
		before   snowflake.ID
	)
	for {
		page, err := client.Rest.GetMessages(b.cfg.RoleChannelID, 0, before, 0, 100)
		if err != nil {
			return nil, err
		}
		for _, message := range page {
			if message.Author.Bot && b.isManagedRolePanelMessage(message) {
				messages = append(messages, message)
			}
		}
		if len(page) < 100 {
			return messages, nil
		}
		before = page[len(page)-1].ID
	}
}

func (b *RoleBot) isManagedRolePanelMessage(message discord.Message) bool {
	for customID := range messageCustomIDs(message) {
		if _, managed := b.roleByCustomID[customID]; managed {
			return true
		}
	}
	return false
}

type customIDSet map[string]struct{}

func (s customIDSet) key() string {
	keys := make([]string, 0, len(s))
	for customID := range s {
		keys = append(keys, customID)
	}
	sort.Strings(keys)
	return strings.Join(keys, "\x00")
}

func messageCustomIDs(message discord.Message) customIDSet {
	return collectButtonCustomIDs(message.Components)
}

func messageCreateCustomIDs(message discord.MessageCreate) customIDSet {
	return collectButtonCustomIDs(message.Components)
}

func collectButtonCustomIDs(components []discord.LayoutComponent) customIDSet {
	customIDs := make(customIDSet)
	for _, layout := range components {
		row, ok := layout.(discord.ActionRowComponent)
		if !ok {
			continue
		}
		for _, component := range row.Components {
			button, ok := component.(discord.ButtonComponent)
			if !ok {
				continue
			}
			customIDs[button.CustomID] = struct{}{}
		}
	}
	return customIDs
}

func indexMessagesByCustomIDs(messages []discord.Message) map[string]snowflake.ID {
	index := make(map[string]snowflake.ID, len(messages))
	for _, message := range messages {
		index[messageCustomIDs(message).key()] = message.ID
	}
	return index
}

func messageUpdateFromCreate(messageCreate discord.MessageCreate) discord.MessageUpdate {
	return discord.NewMessageUpdate().
		WithEmbeds(messageCreate.Embeds...).
		WithComponents(messageCreate.Components...)
}

func totalRoleCount(categories []config.RoleCategory) int {
	count := 0
	for _, category := range categories {
		count += len(category.Roles)
	}
	return count
}
```

- [ ] **Step 3: Create internal/rolepanel/interaction.go**

Moved from `main.go`: `OnReady`, `OnComponentInteraction`, interaction helpers:

```go
package rolepanel

import (
	"log/slog"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"

	"github.com/levtoji/Puerierstab/internal/config"
)

func (b *RoleBot) OnReady(event *events.Ready) {
	b.syncOnce.Do(func() {
		go func() {
			if err := b.publishRolePanels(event.Client()); err != nil {
				slog.Error("publishing role panels failed", slog.Any("err", err))
			}
		}()
	})
}

func (b *RoleBot) OnComponentInteraction(event *events.ComponentInteractionCreate) {
	if event.Data.Type() != discord.ComponentTypeButton {
		return
	}

	button := event.ButtonInteractionData()
	role, ok := b.roleByCustomID[button.CustomID()]
	if !ok {
		return
	}

	guildID := event.GuildID()
	if guildID == nil {
		_ = event.CreateMessage(ephemeralMessage("Diese Rollenverwaltung funktioniert nur auf einem Server."))
		return
	}

	member := event.Member()
	if member == nil {
		_ = event.CreateMessage(ephemeralMessage("Deine Server-Mitgliedschaft konnte nicht geladen werden."))
		return
	}

	userID := userIDFromInteraction(event)
	if userID == 0 {
		_ = event.CreateMessage(ephemeralMessage("Dein Benutzer konnte nicht geladen werden."))
		return
	}

	var (
		err     error
		message string
	)
	if config.MemberHasRole(member.RoleIDs, role.RoleID) {
		err = event.Client().Rest.RemoveMemberRole(*guildID, userID, role.RoleID)
		message = fmtRoleRemoved(role)
	} else {
		err = event.Client().Rest.AddMemberRole(*guildID, userID, role.RoleID)
		message = fmtRoleAdded(role)
	}

	if err != nil {
		slog.Error("role toggle failed", slog.Any("err", err), slog.String("custom_id", role.CustomID), slog.String("role_id", role.RoleID.String()))
		_ = event.CreateMessage(ephemeralMessage("Die Rolle konnte gerade nicht aktualisiert werden. Bitte versuche es erneut."))
		return
	}

	_ = event.CreateMessage(ephemeralMessage(message))
}

func userIDFromInteraction(event *events.ComponentInteractionCreate) snowflake.ID {
	if user := event.User(); user.ID != 0 {
		return user.ID
	}
	if member := event.Member(); member != nil {
		return member.User.ID
	}
	return 0
}

func fmtRoleAdded(role config.RoleButton) string {
	return "Die Rolle „" + role.Label + "“ wurde dir gegeben."
}

func fmtRoleRemoved(role config.RoleButton) string {
	return "Die Rolle „" + role.Label + "“ wurde dir entfernt."
}

func ephemeralMessage(content string) discord.MessageCreate {
	return discord.NewMessageCreate().
		WithContent(content).
		WithEphemeral(true)
}
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor: move rolepanel logic to internal/rolepanel"
```

---

### Task 3: Create rolepanel tests (moved from main_test.go)

**Files:**
- Create: `internal/rolepanel/rolepanel_test.go`

- [ ] **Step 1: Create internal/rolepanel/rolepanel_test.go**

Tests moved from `main_test.go`, adjusted for new package and types:

```go
package rolepanel

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/levtoji/Puerierstab/internal/config"
)

func TestNewRoleBotInitializesRoleMap(t *testing.T) {
	cfg := config.Config{
		Token:         "test_token",
		RoleChannelID: snowflake.MustParse("123456789012345678"),
		Categories: []config.RoleCategory{
			{
				Name:  "Gaming",
				Emoji: "🎮",
				Roles: []config.RoleButton{
					{
						RoleID:   snowflake.MustParse("111111111111111111"),
						Label:    "Minecraft",
						CustomID: "minecraft_toggle",
					},
					{
						RoleID:   snowflake.MustParse("222222222222222222"),
						Label:    "Valorant",
						CustomID: "valorant_toggle",
					},
				},
			},
			{
				Name:  "Films",
				Emoji: "🎬",
				Roles: []config.RoleButton{
					{
						RoleID:   snowflake.MustParse("333333333333333333"),
						Label:    "Filmschauer",
						CustomID: "film_toggle",
					},
				},
			},
		},
	}

	bot := NewRoleBot(cfg)

	if len(bot.roleByCustomID) != 3 {
		t.Fatalf("expected 3 roles, got %d", len(bot.roleByCustomID))
	}

	tests := map[string]string{
		"minecraft_toggle": "Minecraft",
		"valorant_toggle":  "Valorant",
		"film_toggle":      "Filmschauer",
	}

	for customID, expectedLabel := range tests {
		role, ok := bot.roleByCustomID[customID]
		if !ok {
			t.Fatalf("expected role with customID %q to exist", customID)
		}
		if role.Label != expectedLabel {
			t.Fatalf("expected label %q, got %q", expectedLabel, role.Label)
		}
	}
}

func TestTotalRoleCount(t *testing.T) {
	tests := []struct {
		name     string
		category []config.RoleCategory
		expected int
	}{
		{
			"single category single role",
			[]config.RoleCategory{
				{Name: "Cat1", Roles: []config.RoleButton{{Label: "R1"}}},
			},
			1,
		},
		{
			"single category multiple roles",
			[]config.RoleCategory{
				{Name: "Cat1", Roles: []config.RoleButton{
					{Label: "R1"},
					{Label: "R2"},
					{Label: "R3"},
				}},
			},
			3,
		},
		{
			"multiple categories",
			[]config.RoleCategory{
				{Name: "Cat1", Roles: []config.RoleButton{{Label: "R1"}, {Label: "R2"}}},
				{Name: "Cat2", Roles: []config.RoleButton{{Label: "R3"}}},
				{Name: "Cat3", Roles: []config.RoleButton{{Label: "R4"}, {Label: "R5"}, {Label: "R6"}}},
			},
			6,
		},
		{
			"empty categories",
			[]config.RoleCategory{},
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := totalRoleCount(tt.category)
			if result != tt.expected {
				t.Fatalf("expected %d roles, got %d", tt.expected, result)
			}
		})
	}
}

func TestFormatRoleMessages(t *testing.T) {
	tests := []struct {
		name     string
		label    string
		expected string
	}{
		{"simple label", "Filmschauer", "Die Rolle „Filmschauer“ wurde dir gegeben."},
		{"label with spaces", "The Trucker Bros", "Die Rolle „The Trucker Bros“ wurde dir gegeben."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role := config.RoleButton{Label: tt.label}
			result := fmtRoleAdded(role)
			if result != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, result)
			}
		})
	}

	role := config.RoleButton{Label: "TestRole"}
	removeMsg := fmtRoleRemoved(role)
	if removeMsg != "Die Rolle „TestRole“ wurde dir entfernt." {
		t.Fatalf("unexpected removal message: %q", removeMsg)
	}
}

func TestCollectButtonCustomIDs(t *testing.T) {
	tests := []struct {
		name       string
		components []discord.LayoutComponent
		expected   customIDSet
	}{
		{
			"single row single button",
			[]discord.LayoutComponent{
				discord.ActionRowComponent{
					Components: []discord.InteractiveComponent{
						discord.NewPrimaryButton("Film", "film_role_toggle"),
					},
				},
			},
			customIDSet{"film_role_toggle": {}},
		},
		{
			"multiple rows multiple buttons",
			[]discord.LayoutComponent{
				discord.ActionRowComponent{
					Components: []discord.InteractiveComponent{
						discord.NewPrimaryButton("A", "custom_a"),
						discord.NewSuccessButton("B", "custom_b"),
					},
				},
				discord.ActionRowComponent{
					Components: []discord.InteractiveComponent{
						discord.NewDangerButton("C", "custom_c"),
					},
				},
			},
			customIDSet{"custom_a": {}, "custom_b": {}, "custom_c": {}},
		},
		{
			"empty components",
			[]discord.LayoutComponent{},
			customIDSet{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collectButtonCustomIDs(tt.components)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d custom IDs, got %d: %v", len(tt.expected), len(result), result)
			}
			for customID := range tt.expected {
				if _, ok := result[customID]; !ok {
					t.Fatalf("expected custom ID %q in result, got %v", customID, result)
				}
			}
		})
	}
}

func TestCustomIDSetKeyIsDeterministic(t *testing.T) {
	first := customIDSet{"b": {}, "a": {}, "c": {}}
	second := customIDSet{"c": {}, "a": {}, "b": {}}

	if first.key() != second.key() {
		t.Fatalf("expected identical keys for same set, got %q and %q", first.key(), second.key())
	}

	third := customIDSet{"a": {}, "b": {}}
	if first.key() == third.key() {
		t.Fatalf("expected different keys for different sets")
	}
}

func TestIndexMessagesByCustomIDs(t *testing.T) {
	messageA := discord.Message{
		ID:     snowflake.MustParse("111111111111111111"),
		Author: discord.User{ID: snowflake.MustParse("999999999999999999"), Bot: true},
		Components: []discord.LayoutComponent{
			discord.ActionRowComponent{
				Components: []discord.InteractiveComponent{
					discord.NewPrimaryButton("A", "custom_a"),
				},
			},
		},
	}
	messageB := discord.Message{
		ID:     snowflake.MustParse("222222222222222222"),
		Author: discord.User{ID: snowflake.MustParse("999999999999999999"), Bot: true},
		Components: []discord.LayoutComponent{
			discord.ActionRowComponent{
				Components: []discord.InteractiveComponent{
					discord.NewPrimaryButton("B", "custom_b"),
				},
			},
		},
	}

	index := indexMessagesByCustomIDs([]discord.Message{messageA, messageB})

	if len(index) != 2 {
		t.Fatalf("expected 2 entries in index, got %d", len(index))
	}

	keyA := customIDSet{"custom_a": {}}.key()
	if index[keyA] != messageA.ID {
		t.Fatalf("expected message A ID for key %q, got %s", keyA, index[keyA])
	}

	keyB := customIDSet{"custom_b": {}}.key()
	if index[keyB] != messageB.ID {
		t.Fatalf("expected message B ID for key %q, got %s", keyB, index[keyB])
	}
}

func TestIsManagedRolePanelMessage(t *testing.T) {
	bot := NewRoleBot(config.Config{
		Categories: []config.RoleCategory{
			{
				Name: "Filme",
				Roles: []config.RoleButton{
					{RoleID: snowflake.ID(1), Label: "Film", CustomID: "film_role_toggle"},
				},
			},
		},
	})

	managed := discord.Message{
		Author: discord.User{Bot: true},
		Components: []discord.LayoutComponent{
			discord.ActionRowComponent{
				Components: []discord.InteractiveComponent{
					discord.NewPrimaryButton("Film", "film_role_toggle"),
				},
			},
		},
	}
	if !bot.isManagedRolePanelMessage(managed) {
		t.Fatalf("expected message with managed custom ID to be recognized")
	}

	foreign := discord.Message{
		Author: discord.User{Bot: true},
		Components: []discord.LayoutComponent{
			discord.ActionRowComponent{
				Components: []discord.InteractiveComponent{
					discord.NewPrimaryButton("Other", "unrelated_custom_id"),
				},
			},
		},
	}
	if bot.isManagedRolePanelMessage(foreign) {
		t.Fatalf("expected foreign message to not be managed")
	}
}

func TestMessageUpdateFromCreate(t *testing.T) {
	messageCreate := discord.NewMessageCreate().
		WithEmbeds(discord.Embed{Title: "🎮 Gaming", Description: "Test"}).
		AddActionRow(discord.NewPrimaryButton("Film", "film_role_toggle"))

	messageUpdate := messageUpdateFromCreate(messageCreate)

	if messageUpdate.Embeds == nil || len(*messageUpdate.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %v", messageUpdate.Embeds)
	}
	if (*messageUpdate.Embeds)[0].Title != "🎮 Gaming" {
		t.Fatalf("expected embed title, got %q", (*messageUpdate.Embeds)[0].Title)
	}
	if messageUpdate.Components == nil || len(*messageUpdate.Components) != 1 {
		t.Fatalf("expected 1 component, got %v", messageUpdate.Components)
	}
}
```

- [ ] **Step 2: Remove old main_test.go**

```bash
rm main_test.go
```

- [ ] **Step 3: Run rolepanel tests**

```bash
go test ./internal/rolepanel/ -v
```

Expected: all 8 tests pass.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: move rolepanel tests to internal/rolepanel"
```

---

### Task 4: Rewrite main.go as thin entry point

**Files:**
- Rewrite: `main.go`

- [ ] **Step 1: Rewrite main.go**

```go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/gateway"

	"github.com/levtoji/Puerierstab/internal/config"
	"github.com/levtoji/Puerierstab/internal/rolepanel"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if err := run(); err != nil {
		slog.Error("bot stopped", slog.Any("err", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadConfigFromEnv()
	if err != nil {
		return err
	}

	app := rolepanel.NewRoleBot(cfg)

	client, err := disgo.New(cfg.Token,
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(
				gateway.IntentGuilds,
				gateway.IntentGuildMembers,
			),
		),
		bot.WithEventListenerFunc(app.OnReady),
		bot.WithEventListenerFunc(app.OnComponentInteraction),
	)
	if err != nil {
		return err
	}
	defer client.Close(context.Background())

	if err = client.OpenGateway(context.Background()); err != nil {
		return err
	}

	slog.Info("role bot is running", slog.String("version", disgo.Version), slog.String("role_channel_id", cfg.RoleChannelID.String()))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	return nil
}
```

- [ ] **Step 2: Build and run all tests to verify refactor**

```bash
go build ./... && go test ./... -v && go vet ./...
```

Expected: build succeeds, all tests pass, no vet warnings.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "refactor: slim main.go, wire feature packages"
```

---

### Task 5: Add ACTIVITY_LOG_CHANNEL_ID to config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestLoadActivityLogChannelIDSet(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "token")
	t.Setenv(roleChannelEnv, "123456789012345678")
	t.Setenv(roleConfigEnv, `[{"name":"Test","emoji":"🎮","roles":[{"label":"R1","role_id":"1"}]}]`)
	t.Setenv("ACTIVITY_LOG_CHANNEL_ID", "1536346100909473792")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := snowflake.MustParse("1536346100909473792")
	if cfg.ActivityLogChannelID != want {
		t.Fatalf("expected ActivityLogChannelID %s, got %s", want, cfg.ActivityLogChannelID)
	}
}

func TestLoadActivityLogChannelIDUnset(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "token")
	t.Setenv(roleChannelEnv, "123456789012345678")
	t.Setenv(roleConfigEnv, `[{"name":"Test","emoji":"🎮","roles":[{"label":"R1","role_id":"1"}]}]`)
	// ACTIVITY_LOG_CHANNEL_ID not set

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ActivityLogChannelID != 0 {
		t.Fatalf("expected ActivityLogChannelID 0 (unset), got %s", cfg.ActivityLogChannelID)
	}
}

func TestLoadActivityLogChannelIDInvalid(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "token")
	t.Setenv(roleChannelEnv, "123456789012345678")
	t.Setenv(roleConfigEnv, `[{"name":"Test","emoji":"🎮","roles":[{"label":"R1","role_id":"1"}]}]`)
	t.Setenv("ACTIVITY_LOG_CHANNEL_ID", "not_a_number")

	_, err := LoadConfigFromEnv()
	if err == nil {
		t.Fatalf("expected error for invalid ACTIVITY_LOG_CHANNEL_ID, got nil")
	}
	if !strings.Contains(err.Error(), "invalid snowflake") {
		t.Fatalf("expected error to mention invalid snowflake, got %q", err.Error())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/config/ -run TestLoadActivityLogChannelID -v
```

Expected: FAIL — `ActivityLogChannelID unknown field` or the field doesn't exist yet.

- [ ] **Step 3: Implement the config changes**

In `internal/config/config.go`, add after existing constants:

```go
const activityLogChannelEnv = "ACTIVITY_LOG_CHANNEL_ID"
```

Add field to `Config` struct:

```go
type Config struct {
	Token               string
	RoleChannelID       snowflake.ID
	ActivityLogChannelID snowflake.ID
	Categories          []RoleCategory
}
```

In `LoadConfigFromEnv`, before the return, after loading categories:

```go
	activityLogChannelID, err := parseOptionalSnowflakeEnv(activityLogChannelEnv)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", activityLogChannelEnv, err)
	}

	return Config{
		Token:               token,
		RoleChannelID:       roleChannelID,
		ActivityLogChannelID: activityLogChannelID,
		Categories:          categories,
	}, nil
```

Add new helper function before `parseSnowflakeEnv`:

```go
func parseOptionalSnowflakeEnv(key string) (snowflake.ID, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0, nil
	}
	return parseSnowflake(value)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/config/ -v
```

Expected: all tests pass (14 tests now — 11 existing + 3 new).

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: add optional ACTIVITY_LOG_CHANNEL_ID to config"
```

---

### Task 6: Create activitylog embed builders and diff helpers

**Files:**
- Create: `internal/activitylog/activitylog.go`
- Create: `internal/activitylog/activitylog_test.go`

- [ ] **Step 1: Create directory**

```bash
mkdir -p internal/activitylog
```

- [ ] **Step 2: Write the failing tests for embed builders**

Create `internal/activitylog/activitylog_test.go`:

```go
package activitylog

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func effectiveName(name string) discord.Member {
	return discord.Member{
		Nick: &name,
		User: discord.User{Username: "backup", GlobalName: &name},
	}
}

func effectiveNameGlobal(name string) discord.Member {
	return discord.Member{
		User: discord.User{Username: "backup", GlobalName: &name},
	}
}

func memberNoNick(username string) discord.Member {
	return discord.Member{
		Nick: nil,
		User: discord.User{Username: username},
	}
}

func TestJoinEmbed(t *testing.T) {
	member := effectiveName("Alice")
	embed := joinEmbed(member)

	if embed.Title != "**Alice** ist beigetreten" {
		t.Fatalf("expected title %q, got %q", "**Alice** ist beigetreten", embed.Title)
	}
	if embed.Color != 0x57F287 {
		t.Fatalf("expected color 0x57F287, got 0x%X", embed.Color)
	}
}

func TestLeaveEmbed(t *testing.T) {
	member := effectiveName("Bob")
	embed := leaveEmbed(member)

	if embed.Title != "**Bob** hat den Server verlassen" {
		t.Fatalf("expected leave title, got %q", embed.Title)
	}
	if embed.Color != 0xED4245 {
		t.Fatalf("expected color 0xED4245, got 0x%X", embed.Color)
	}
}

func TestNickChangeEmbed(t *testing.T) {
	member := effectiveName("NewName")
	embed := nickChangeEmbed(member, "OldName", "NewName")

	if embed.Title != "**NewName**: OldName → NewName" {
		t.Fatalf("expected nick change title, got %q", embed.Title)
	}
	if embed.Color != 0x95A5A6 {
		t.Fatalf("expected color 0x95A5A6, got 0x%X", embed.Color)
	}
}

func TestRoleAddedEmbed(t *testing.T) {
	member := effectiveName("Charlie")
	embed := roleAddedEmbed(member, "Trucker")

	if embed.Title != "**Charlie** + Trucker" {
		t.Fatalf("expected role added title, got %q", embed.Title)
	}
	if embed.Color != 0x5865F2 {
		t.Fatalf("expected color 0x5865F2, got 0x%X", embed.Color)
	}
}

func TestRoleRemovedEmbed(t *testing.T) {
	member := effectiveName("Diana")
	embed := roleRemovedEmbed(member, "Filmschauer")

	if embed.Title != "**Diana** − Filme" {
		t.Fatalf("expected role removed title, got %q", embed.Title)
	}
	if embed.Color != 0xE67E22 {
		t.Fatalf("expected color 0xE67E22, got 0x%X", embed.Color)
	}
}

func TestVoiceJoinEmbed(t *testing.T) {
	member := effectiveName("Eve")
	embed := voiceJoinEmbed(member, "Allgemein")

	if embed.Title != "**Eve** → #Allgemein" {
		t.Fatalf("expected voice join title, got %q", embed.Title)
	}
	if embed.Color != 0x3498DB {
		t.Fatalf("expected color 0x3498DB, got 0x%X", embed.Color)
	}
}

func TestVoiceLeaveEmbed(t *testing.T) {
	member := effectiveName("Frank")
	embed := voiceLeaveEmbed(member, "Gaming")

	if embed.Title != "**Frank** ← #Gaming" {
		t.Fatalf("expected voice leave title, got %q", embed.Title)
	}
	if embed.Color != 0x3498DB {
		t.Fatalf("expected color 0x3498DB, got 0x%X", embed.Color)
	}
}

func TestVoiceMoveEmbed(t *testing.T) {
	member := effectiveName("Grace")
	embed := voiceMoveEmbed(member, "Allgemein", "Gaming")

	if embed.Title != "**Grace**: #Allgemein → #Gaming" {
		t.Fatalf("expected voice move title, got %q", embed.Title)
	}
	if embed.Color != 0x3498DB {
		t.Fatalf("expected color 0x3498DB, got 0x%X", embed.Color)
	}
}

func TestNickDiff(t *testing.T) {
	old := effectiveName("OldNick")
	new := effectiveName("NewNick")

	oldName, newName, changed := nickDiff(old, new)
	if !changed {
		t.Fatalf("expected changed=true when nicks differ")
	}
	if oldName != "OldNick" || newName != "NewNick" {
		t.Fatalf("expected OldNick→NewNick, got %s→%s", oldName, newName)
	}

	// Same nick
	same := effectiveName("Same")
	_, _, changed = nickDiff(same, same)
	if changed {
		t.Fatalf("expected changed=false when nicks are the same")
	}

	// From no nick (username only) to nick
	noNick := memberNoNick("username")
	withNick := effectiveName("WithNick")
	oldName, newName, changed = nickDiff(noNick, withNick)
	if !changed {
		t.Fatalf("expected changed=true when nick was added")
	}
	if oldName != "username" || newName != "WithNick" {
		t.Fatalf("expected username→WithNick, got %s→%s", oldName, newName)
	}

	// Global name (no nickname set on either)
	oldGlobal := effectiveNameGlobal("OldGlobal")
	newGlobal := effectiveNameGlobal("NewGlobal")
	oldName, newName, changed = nickDiff(oldGlobal, newGlobal)
	if !changed {
		t.Fatalf("expected changed=true when global names differ")
	}
	if oldName != "OldGlobal" || newName != "NewGlobal" {
		t.Fatalf("expected OldGlobal→NewGlobal, got %s→%s", oldName, newName)
	}
}

func TestRoleDiff(t *testing.T) {
	roleA := snowflake.ID(1)
	roleB := snowflake.ID(2)
	roleC := snowflake.ID(3)

	// No changes
	added, removed := roleDiff([]snowflake.ID{roleA, roleB}, []snowflake.ID{roleA, roleB})
	if len(added) != 0 || len(removed) != 0 {
		t.Fatalf("expected no changes, got added=%v removed=%v", added, removed)
	}

	// Role added
	added, removed = roleDiff([]snowflake.ID{roleA}, []snowflake.ID{roleA, roleB})
	if len(added) != 1 || len(removed) != 0 {
		t.Fatalf("expected 1 added, got added=%v removed=%v", added, removed)
	}
	if added[0] != roleB {
		t.Fatalf("expected roleB added, got %s", added[0])
	}

	// Role removed
	added, removed = roleDiff([]snowflake.ID{roleA, roleB}, []snowflake.ID{roleA})
	if len(added) != 0 || len(removed) != 1 {
		t.Fatalf("expected 1 removed, got added=%v removed=%v", added, removed)
	}
	if removed[0] != roleB {
		t.Fatalf("expected roleB removed, got %s", removed[0])
	}

	// Both added and removed
	added, removed = roleDiff([]snowflake.ID{roleA, roleB}, []snowflake.ID{roleA, roleC})
	if len(added) != 1 || len(removed) != 1 {
		t.Fatalf("expected 1 added + 1 removed, got added=%v removed=%v", added, removed)
	}
	if added[0] != roleC || removed[0] != roleB {
		t.Fatalf("expected C added, B removed, got added=%v removed=%v", added, removed)
	}

	// Empty to something
	added, removed = roleDiff(nil, []snowflake.ID{roleA})
	if len(added) != 1 || len(removed) != 0 {
		t.Fatalf("expected 1 added from nil, got added=%v removed=%v", added, removed)
	}

	// Something to empty
	added, removed = roleDiff([]snowflake.ID{roleA}, nil)
	if len(added) != 0 || len(removed) != 1 {
		t.Fatalf("expected 1 removed, got added=%v removed=%v", added, removed)
	}
}

func TestMemberName(t *testing.T) {
	// Nick present → uses nick
	member := discord.Member{
		Nick: ptr("Nickname"),
		User: discord.User{Username: "Username", GlobalName: ptr("Global")},
	}
	if memberName(member) != "Nickname" {
		t.Fatalf("expected Nickname, got %q", memberName(member))
	}

	// No nick → falls back to global name
	member = discord.Member{
		Nick: nil,
		User: discord.User{Username: "Username", GlobalName: ptr("Global")},
	}
	if memberName(member) != "Global" {
		t.Fatalf("expected Global, got %q", memberName(member))
	}

	// No nick, no global → falls back to username
	member = discord.Member{
		Nick: nil,
		User: discord.User{Username: "Username"},
	}
	if memberName(member) != "Username" {
		t.Fatalf("expected Username, got %q", memberName(member))
	}
}

func ptr[T any](v T) *T { return &v }
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/activitylog/ -v
```

Expected: FAIL — `joinEmbed undefined`, etc.

- [ ] **Step 4: Implement embed builders and diff helpers**

Create `internal/activitylog/activitylog.go`:

```go
package activitylog

import (
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

type ActivityLog struct {
	channelID snowflake.ID
}

func New(channelID snowflake.ID) *ActivityLog {
	return &ActivityLog{channelID: channelID}
}

// ---- Embed builders ----

func joinEmbed(member discord.Member) discord.Embed {
	return discord.Embed{
		Title: fmt.Sprintf("**%s** ist beigetreten", memberName(member)),
		Color: 0x57F287,
	}
}

func leaveEmbed(member discord.Member) discord.Embed {
	return discord.Embed{
		Title: fmt.Sprintf("**%s** hat den Server verlassen", memberName(member)),
		Color: 0xED4245,
	}
}

func nickChangeEmbed(member discord.Member, oldName, newName string) discord.Embed {
	return discord.Embed{
		Title: fmt.Sprintf("**%s**: %s → %s", memberName(member), oldName, newName),
		Color: 0x95A5A6,
	}
}

func roleAddedEmbed(member discord.Member, roleName string) discord.Embed {
	return discord.Embed{
		Title: fmt.Sprintf("**%s** + %s", memberName(member), roleName),
		Color: 0x5865F2,
	}
}

func roleRemovedEmbed(member discord.Member, roleName string) discord.Embed {
	return discord.Embed{
		Title: fmt.Sprintf("**%s** − %s", memberName(member), roleName),
		Color: 0xE67E22,
	}
}

func voiceJoinEmbed(member discord.Member, channelName string) discord.Embed {
	return discord.Embed{
		Title: fmt.Sprintf("**%s** → #%s", memberName(member), channelName),
		Color: 0x3498DB,
	}
}

func voiceLeaveEmbed(member discord.Member, channelName string) discord.Embed {
	return discord.Embed{
		Title: fmt.Sprintf("**%s** ← #%s", memberName(member), channelName),
		Color: 0x3498DB,
	}
}

func voiceMoveEmbed(member discord.Member, fromChannel, toChannel string) discord.Embed {
	return discord.Embed{
		Title: fmt.Sprintf("**%s**: #%s → #%s", memberName(member), fromChannel, toChannel),
		Color: 0x3498DB,
	}
}

// ---- Helpers ----

func memberName(member discord.Member) string {
	return member.EffectiveName()
}

func nickDiff(oldMember, newMember discord.Member) (oldName, newName string, changed bool) {
	oldName = oldMember.EffectiveName()
	newName = newMember.EffectiveName()
	return oldName, newName, oldName != newName
}

func roleDiff(oldIDs, newIDs []snowflake.ID) (added, removed []snowflake.ID) {
	old := make(map[snowflake.ID]struct{}, len(oldIDs))
	for _, id := range oldIDs {
		old[id] = struct{}{}
	}
	nu := make(map[snowflake.ID]struct{}, len(newIDs))
	for _, id := range newIDs {
		nu[id] = struct{}{}
	}

	for _, id := range newIDs {
		if _, ok := old[id]; !ok {
			added = append(added, id)
		}
	}
	for _, id := range oldIDs {
		if _, ok := nu[id]; !ok {
			removed = append(removed, id)
		}
	}
	return added, removed
}

func (l *ActivityLog) roleName(client *bot.Client, guildID, roleID snowflake.ID) string {
	if role, ok := client.Caches.Role(guildID, roleID); ok && role.Name != "" {
		return role.Name
	}
	return roleID.String()
}

func (l *ActivityLog) channelName(client *bot.Client, channelID snowflake.ID) string {
	if channel, ok := client.Caches.GuildVoiceChannel(channelID); ok {
		return channel.Name()
	}
	return channelID.String()
}

func (l *ActivityLog) post(client *bot.Client, embed discord.Embed) {
	_, err := client.Rest.CreateMessage(l.channelID, discord.NewMessageCreate().WithEmbeds(embed))
	if err != nil {
		slog.Warn("failed to post activity log", slog.Any("err", err))
	}
}

// ---- Event handlers ----

func (l *ActivityLog) OnGuildMemberJoin(event *events.GuildMemberJoin) {
	l.post(event.Client(), joinEmbed(event.Member))
}

func (l *ActivityLog) OnGuildMemberLeave(event *events.GuildMemberLeave) {
	l.post(event.Client(), leaveEmbed(event.Member))
}

func (l *ActivityLog) OnGuildMemberUpdate(event *events.GuildMemberUpdate) {
	oldName, newName, changed := nickDiff(event.OldMember, event.Member)
	if changed {
		l.post(event.Client(), nickChangeEmbed(event.Member, oldName, newName))
	}

	added, removed := roleDiff(event.OldMember.RoleIDs, event.Member.RoleIDs)
	for _, roleID := range added {
		l.post(event.Client(), roleAddedEmbed(event.Member, l.roleName(event.Client(), event.GuildID, roleID)))
	}
	for _, roleID := range removed {
		l.post(event.Client(), roleRemovedEmbed(event.Member, l.roleName(event.Client(), event.GuildID, roleID)))
	}
}

func (l *ActivityLog) OnGuildVoiceJoin(event *events.GuildVoiceJoin) {
	if event.VoiceState.ChannelID == nil {
		return
	}
	l.post(event.Client(), voiceJoinEmbed(event.Member, l.channelName(event.Client(), *event.VoiceState.ChannelID)))
}

func (l *ActivityLog) OnGuildVoiceMove(event *events.GuildVoiceMove) {
	from := event.OldVoiceState.ChannelID
	to := event.VoiceState.ChannelID
	if from == nil || to == nil {
		return
	}
	l.post(event.Client(), voiceMoveEmbed(event.Member, l.channelName(event.Client(), *from), l.channelName(event.Client(), *to)))
}

func (l *ActivityLog) OnGuildVoiceLeave(event *events.GuildVoiceLeave) {
	from := event.OldVoiceState.ChannelID
	if from == nil {
		return
	}
	l.post(event.Client(), voiceLeaveEmbed(event.Member, l.channelName(event.Client(), *from)))
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/activitylog/ -v
```

Expected: all 11 tests pass.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: add activitylog embed builders, diff helpers, and event handlers"
```

---

### Task 7: Wire activitylog listeners in main.go

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Add activitylog import and conditional wiring**

Replace `main.go` with:

```go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/gateway"

	"github.com/levtoji/Puerierstab/internal/activitylog"
	"github.com/levtoji/Puerierstab/internal/config"
	"github.com/levtoji/Puerierstab/internal/rolepanel"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if err := run(); err != nil {
		slog.Error("bot stopped", slog.Any("err", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadConfigFromEnv()
	if err != nil {
		return err
	}

	app := rolepanel.NewRoleBot(cfg)

	intents := []gateway.Intents{gateway.IntentGuilds, gateway.IntentGuildMembers}
	listeners := []bot.ConfigOpt{
		bot.WithEventListenerFunc(app.OnReady),
		bot.WithEventListenerFunc(app.OnComponentInteraction),
	}

	if cfg.ActivityLogChannelID != 0 {
		intents = append(intents, gateway.IntentGuildVoiceStates)
		logger := activitylog.New(cfg.ActivityLogChannelID)
		listeners = append(listeners,
			bot.WithEventListenerFunc(logger.OnGuildMemberJoin),
			bot.WithEventListenerFunc(logger.OnGuildMemberLeave),
			bot.WithEventListenerFunc(logger.OnGuildMemberUpdate),
			bot.WithEventListenerFunc(logger.OnGuildVoiceJoin),
			bot.WithEventListenerFunc(logger.OnGuildVoiceMove),
			bot.WithEventListenerFunc(logger.OnGuildVoiceLeave),
		)
	}

	opts := []bot.ConfigOpt{
		bot.WithGatewayConfigOpts(gateway.WithIntents(intents...)),
	}
	opts = append(opts, listeners...)

	client, err := disgo.New(cfg.Token, opts...)
	if err != nil {
		return err
	}
	defer client.Close(context.Background())

	if err = client.OpenGateway(context.Background()); err != nil {
		return err
	}

	slog.Info("role bot is running", slog.String("version", disgo.Version), slog.String("role_channel_id", cfg.RoleChannelID.String()))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	return nil
}
```

- [ ] **Step 2: Build, test, vet**

```bash
go build ./... && go test ./... -v && go vet ./...
```

Expected: build succeeds, all tests pass, no vet warnings.

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "feat: wire activitylog listeners in main with IntentGuildVoiceStates"
```

---
