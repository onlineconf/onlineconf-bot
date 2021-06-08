package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/onlineconf/onlineconf-go"
	"github.com/rs/zerolog/log"
)

var commandName = filepath.Base(flag.CommandLine.Name())
var configDir = flag.String("config-dir", "", "onlineconf configuration files directory")
var configModule = flag.String("config-module", commandName, "onlineconf module name")

var config *onlineconf.Module
var db *database

func main() {
	flag.Parse()
	if *configDir != "" {
		onlineconf.Initialize(*configDir)
	}
	config = onlineconf.GetModule(*configModule)
	var err error
	db, err = openDatabase()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open database")
	}
	err = db.Ping()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to ping database")
	}
	bot, err := newBot()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize Myteam bot")
	}

	ctx, cancel := context.WithCancel(log.Logger.WithContext(context.Background()))
	defer cancel()

	log.Info().Msg("onlineconf-myteam-bot started")
	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigC)
	go func() {
		sig := <-sigC
		log.Info().Str("signal", sig.String()).Msg("signal received, terminating")
		signal.Stop(sigC)
		cancel()
	}()

	go bot.updatesProcessor(ctx)
	notificationsReceiver(ctx, bot)

	log.Info().Msg("onlineconf-myteam-bot stopped")
}
