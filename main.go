package onlineconfbot

import (
	"context"
	"flag"
	stdlog "log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/onlineconf/onlineconf-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var commandName = filepath.Base(flag.CommandLine.Name())
var configDir = flag.String("config-dir", "", "onlineconf configuration files directory")
var configModule = flag.String("config-module", commandName, "onlineconf module name")
var logLevel = flag.String("log-level", "debug", "log level")

var config *onlineconf.Module
var db *database

func BotMain[botType Bot](newBot func(*onlineconf.Module, SubscriptionStorage) (botType, error)) {
	flag.Parse()

	stdlog.SetFlags(0)
	onlineconf.SetOutput(log.Logger)

	level, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		log.Fatal().Err(err).Msg("unknown log level specified")
	}

	zerolog.SetGlobalLevel(level)

	if *configDir != "" {
		onlineconf.Initialize(*configDir)
	}
	config = onlineconf.GetModule(*configModule)
	db, err = openDatabase()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open database")
	}
	err = db.Ping()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to ping database")
	}
	bot, err := newBot(config, db)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize bot")
	}

	ctx, cancel := context.WithCancel(log.Logger.WithContext(context.Background()))
	defer cancel()

	log.Info().Msg("onlineconf-bot started")
	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigC)
	go func() {
		sig := <-sigC
		log.Info().Str("signal", sig.String()).Msg("signal received, terminating")
		signal.Stop(sigC)
		cancel()
	}()

	go bot.UpdatesProcessor(ctx)
	notificationsReceiver(ctx, bot)

	log.Info().Msg("onlineconf-bot stopped")
}
