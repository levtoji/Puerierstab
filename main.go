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
