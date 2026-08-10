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
