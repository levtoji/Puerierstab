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
	"github.com/levtoji/Puerierstab/internal/asciireact"
	"github.com/levtoji/Puerierstab/internal/channelnamer"
	"github.com/levtoji/Puerierstab/internal/chatlog"
	"github.com/levtoji/Puerierstab/internal/config"
	"github.com/levtoji/Puerierstab/internal/icebreaker"
	"github.com/levtoji/Puerierstab/internal/memereact"
	"github.com/levtoji/Puerierstab/internal/poll"
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
	pollStore = poll.NewStore()
	reactor := asciireact.New()
	stopCleanup := pollStore.StartCleanup()
	defer close(stopCleanup)

	chatLog = chatlog.New()
	stopChatCleanup := chatLog.StartCleanup()
	defer close(stopChatCleanup)

	ibHandler, err := icebreaker.NewHandler()
	if err != nil {
		return err
	}
	icebreakerHandler = ibHandler

	namer := channelnamer.New(channelnamer.Config{
		ChannelIDs:   cfg.RenameChannelIDs,
		APIKey:       cfg.AIAPIKey,
		Model:        cfg.AIModel,
		BaseURL:      cfg.AIBaseURL,
		LogChannelID: cfg.ActivityLogChannelID,
	})
	channelNamer = namer

	aiAPIKey = cfg.AIAPIKey
	aiModel = cfg.AIModel
	aiBaseURL = cfg.AIBaseURL

	memeReactor = memereact.New(memereact.Config{
		AIAPIKey:    cfg.AIAPIKey,
		AIModel:     cfg.AIModel,
		AIBaseURL:   cfg.AIBaseURL,
		GiphyAPIKey: cfg.GiphyAPIKey,
	})

	intents := []gateway.Intents{gateway.IntentGuilds, gateway.IntentGuildMembers, gateway.IntentMessageContent, gateway.IntentGuildMessageReactions}
	listeners := []bot.ConfigOpt{
		bot.WithEventListenerFunc(app.OnReady),
		bot.WithEventListenerFunc(registerCommandsOnReady),
		bot.WithEventListenerFunc(app.OnComponentInteraction),
		bot.WithEventListenerFunc(pollStore.HandleComponent),
		bot.WithEventListenerFunc(reactor.OnMessageCreate),
		bot.WithEventListenerFunc(chatLog.OnMessageCreate),
		bot.WithEventListenerFunc(handleSlashCommand),
	}

	if memeReactor != nil {
		listeners = append(listeners, bot.WithEventListenerFunc(memeReactor.OnReactionAdd))
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

	if namer != nil {
		slog.Info("channel namer active", slog.Int("channels", len(cfg.RenameChannelIDs)))
		stopNamer := namer.Start(client)
		defer close(stopNamer)
	} else if len(cfg.RenameChannelIDs) > 0 {
		slog.Warn("channel namer disabled — AI_API_KEY missing")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	return nil
}
