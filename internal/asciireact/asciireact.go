package asciireact

import (
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
)

var reactions = map[string]string{
	"hunde":       `ʕ•ᴥ•ʔ`,
	"hund":        `ʕ•ᴥ•ʔ`,
	"doggo":       `ʕ•ᴥ•ʔ`,
	"dog":         `ʕ•ᴥ•ʔ`,
	"katze":       `ᓚᘏᗢ`,
	"cat":         `ᓚᘏᗢ`,
	"miau":        `ᓚᘏᗢ`,
	"party":       "   (\\_/)\n   ( •_•)\n   />🎉>\n",
	"feier":       "   (\\_/)\n   ( •_•)\n   />🎉>\n",
	"feiern":      "   (\\_/)\n   ( •_•)\n   />🎉>\n",
	"montag":      "   ╔══╗\n   ║▓▓║  ☕\n   ║▓▓║\n   ╚══╝",
	"pizza":       "      _\n     (_)\n     /_\\\n    o| |o\n   __|_|__\n  | [] [] |\n  |_______|",
	"kaffee":      "   ( (\n    ) )\n  .______.\n  |      |\n  |      |\n  '------'",
	"schlafen":    "   ᶻ 𝗓 𐰁\n  |\\__/|\n /  •ᴗ• \\\n |______|",
	"müde":        "   ᶻ 𝗓 𐰁\n  |\\__/|\n /  •ᴗ• \\\n |______|",
	"bett":        "   ᶻ 𝗓 𐰁\n  |\\__/|\n /  •ᴗ• \\\n |______|",
	"code":        "  ┌─┐\n  │┼│\n  └─┘\n ▐▓█▌\n  ███",
	"programmier": "  ┌─┐\n  │┼│\n  └─┘\n ▐▓█▌\n  ███",
	"bug":         "  ┌─┐\n  │🐛│\n  └─┘\n ▐▓█▌\n  ███",
	"regen":       "  .-.\n (   )  ☂\n  '-'",
	"wetter":      "  .-.\n (   )  ☂\n  '-'",
	"discord":     "{◕ ◡ ◕}",
	"nitro":       "{◕ ◡ ◕}",
	"gaming":      "   .====.\n   |[OK]|\n   |    |\n   '===='",
	"zock":        "   .====.\n   |[OK]|\n   |    |\n   '===='",
	"spiel":       "   .====.\n   |[OK]|\n   |    |\n   '===='",
	"musik":       "  ♪♪\n (⌐■_■)\n  /||\\\n   ||",
	"banane":      "   ___((\n  /____\\_)_\n  \\_____/",
	"banan":       "   ___((\n  /____\\_)_\n  \\_____/",
}

type Reactor struct {
	lastFire time.Time
	mu       sync.Mutex
}

func New() *Reactor {
	return &Reactor{}
}

func (r *Reactor) matchKeyword(content string) (string, bool) {
	lower := strings.ToLower(content)
	for keyword, art := range reactions {
		if strings.Contains(lower, keyword) {
			return art, true
		}
	}
	return "", false
}

func (r *Reactor) OnMessageCreate(event *events.GuildMessageCreate) {
	if event.Message.Author.Bot {
		return
	}

	art, ok := r.matchKeyword(event.Message.Content)
	if !ok {
		return
	}

	r.mu.Lock()
	if time.Since(r.lastFire) < 10*time.Second {
		r.mu.Unlock()
		return
	}
	r.lastFire = time.Now()
	r.mu.Unlock()

	_, _ = event.Client().Rest.CreateMessage(event.ChannelID, discord.NewMessageCreate().
		WithContent("```\n" + art + "\n```"))
}
