package main

import (
	"context"
	"net/url"
	"strings"

	botgolang "github.com/mail-ru-im/bot-golang"
	"github.com/rs/zerolog/log"
)

type myteamBot struct {
	*botgolang.Bot
}

func newBot() (*myteamBot, error) {
	var opts []botgolang.BotOption
	if url := config.GetString("/myteam/url", ""); url != "" {
		opts = append(opts, botgolang.BotApiURL(url))
	}
	opts = append(opts, botgolang.BotDebug(true))
	bot, err := botgolang.NewBot(config.GetString("/myteam/token", ""), opts...)
	if err != nil {
		return nil, err
	}
	return &myteamBot{bot}, nil
}

func (bot *myteamBot) updatesProcessor(ctx context.Context) {
	for event := range bot.GetUpdatesChannel(ctx) {
		switch event.Type {
		case botgolang.NEW_MESSAGE, botgolang.EDITED_MESSAGE:
			switch event.Payload.Text {
			case "/start":
				err := bot.sendSubscribePrompt(event.Payload.From.ID)
				if err != nil {
					log.Ctx(ctx).Error().Err(err).Msg("failed to send subscribe prompt")
				}
			case "/stop":
				err := bot.unsubscribe(ctx, event.Payload.From.ID)
				if err != nil {
					log.Ctx(ctx).Error().Err(err).Msg("failed to unsubscribe")
				}
			case "/subscribers":
				if isAdmin(event.Payload.From.ID) {
					err := bot.sendSubscribers(ctx, event.Payload.From.ID)
					if err != nil {
						log.Ctx(ctx).Error().Err(err).Msg("failed to send subscribes")
					}
				}
			}
		case botgolang.CALLBACK_QUERY:
			var err error
			switch event.Payload.CallbackData {
			case "subscribe write":
				err = bot.subscribe(ctx, event.Payload.From.ID, true)
			case "subscribe read":
				err = bot.subscribe(ctx, event.Payload.From.ID, false)
			}
			responseText := ""
			if err != nil {
				log.Ctx(ctx).Error().Err(err).Msg("failed to subscribe")
				responseText = "Internal error"
			}
			response := bot.NewButtonResponse(event.Payload.QueryID, "", responseText, false)
			response.Send()
		}
	}
}

func (bot *myteamBot) sendSubscribePrompt(user string) error {
	message := bot.NewInlineKeyboardMessage(
		user,
		"Choose parameters you want to subscribe to",
		[][]botgolang.Button{{
			{Text: "I can edit", CallbackData: "subscribe write"},
			{Text: "I can view", CallbackData: "subscribe read"},
		}},
	)
	return message.Send()
}

func (bot *myteamBot) subscribe(ctx context.Context, user string, wo bool) error {
	err := db.Subscribe(ctx, user, wo)
	if err != nil {
		return err
	}
	var which string
	if wo {
		which = "edit"
	} else {
		which = "view"
	}
	message := bot.NewTextMessage(user, "You subscribed to parameters you can "+which)
	return message.Send()
}

func (bot *myteamBot) unsubscribe(ctx context.Context, user string) error {
	err := db.Unsubscribe(ctx, user)
	if err != nil {
		return err
	}
	message := bot.NewTextMessage(user, "You unsubscribed")
	return message.Send()
}

func (bot *myteamBot) sendSubscribers(ctx context.Context, user string) error {
	subscribers, err := db.Subscribers(ctx)
	if err != nil {
		return err
	}
	text := strings.Builder{}
	for user, wo := range subscribers {
		text.WriteString("@[")
		text.WriteString(user)
		text.WriteString("] ")
		if wo {
			text.WriteString("edit")
		} else {
			text.WriteString("view")
		}
		text.WriteString("\n")
	}
	message := bot.NewTextMessage(user, text.String())
	return message.Send()
}

func (bot *myteamBot) notify(ctx context.Context, notification Notification) error {
	users := map[string]string{}
	domain := config.GetString("/user/domain", "")
	var userMap map[string]string
	config.GetStruct("/user/map", &userMap)
	for user, access := range notification.Users {
		myteamUser := userMap[user]
		if myteamUser == "" {
			myteamUser = user
		}
		if domain != "" {
			myteamUser += "@" + domain
		}
		users[myteamUser] = access
	}

	notifyUsers, err := db.FilterSubscribed(ctx, users)
	if err != nil {
		return err
	}
	if len(notifyUsers) == 0 {
		return nil
	}

	text := notification.Text()

	var keyboard [][]botgolang.Button
	if linkUrl, err := url.ParseRequestURI(config.GetString("/onlineconf/link-url")); err == nil {
		linkUrl.Fragment = notification.Path
		keyboard = [][]botgolang.Button{{{Text: "Open", URL: linkUrl.String()}}}
	} else {
		log.Ctx(ctx).Warn().Err(err).Msg("failed to parse link URL")
	}

	for _, user := range notifyUsers {
		message := bot.NewInlineKeyboardMessage(user, text, keyboard)
		err := message.Send()
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("failed to send notification")
		}
	}
	return nil
}

func myteamUser(user string) string {
	var userMap map[string]string
	config.GetStruct("/user/map", &userMap)
	domain := config.GetString("/user/domain", "")
	myteamUser := userMap[user]
	if myteamUser == "" {
		myteamUser = user
	}
	if domain != "" {
		myteamUser += "@" + domain
	}
	return myteamUser
}

func isAdmin(user string) bool {
	for _, u := range config.GetStrings("/user/admins", nil) {
		if u == user {
			return true
		}
	}
	return false
}
