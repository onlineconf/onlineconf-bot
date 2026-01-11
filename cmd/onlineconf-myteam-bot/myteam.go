package main

import (
	"context"
	"crypto/tls"
	"net/http"
	"strings"

	botgolang "github.com/mail-ru-im/bot-golang"
	onlineconfbot "github.com/onlineconf/onlineconf-bot"
	"github.com/onlineconf/onlineconf-go"
	"github.com/rs/zerolog/log"
)

type MyteamBot struct {
	*botgolang.Bot
	subscr onlineconfbot.SubscriptionStorage
	id     int
}

var _ onlineconfbot.Bot = MyteamBot{}

func NewMyteamBot(config *onlineconf.Module, subscr onlineconfbot.SubscriptionStorage) (MyteamBot, error) {
	var opts []botgolang.BotOption
	if url := config.GetString("/myteam/url", ""); url != "" {
		opts = append(opts, botgolang.BotApiURL(url))
	}
	if config.GetBool("/myteam/debug", false) {
		opts = append(opts, botgolang.BotDebug(true))
	}
	if config.GetBool("/myteam/insecure", false) {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		c := http.Client{Transport: tr}
		opts = append(opts, botgolang.BotHTTPClient(c))
	}

	bot, err := botgolang.NewBot(config.GetString("/myteam/token", ""), opts...)
	if err != nil {
		return MyteamBot{}, err
	}
	id := config.GetInt("/myteam/id", 0)
	return MyteamBot{bot, subscr, id}, nil
}

func (bot MyteamBot) UpdatesProcessor(ctx context.Context) {
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
				if onlineconfbot.IsAdmin(event.Payload.From.ID) {
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

func (bot MyteamBot) sendSubscribePrompt(user string) error {
	message := bot.NewInlineKeyboardMessage(
		user,
		"Choose parameters you want to subscribe to",
		botgolang.Keyboard{
			Rows: [][]botgolang.Button{{
				{Text: "I can edit", CallbackData: "subscribe write"},
				{Text: "I can view", CallbackData: "subscribe read"},
			}}},
	)
	return message.Send()
}

func (bot MyteamBot) subscribe(ctx context.Context, user string, wo bool) error {
	err := bot.subscr.Subscribe(ctx, user, wo)
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

func (bot MyteamBot) unsubscribe(ctx context.Context, user string) error {
	err := bot.subscr.Unsubscribe(ctx, user)
	if err != nil {
		return err
	}
	message := bot.NewTextMessage(user, "You unsubscribed")
	return message.Send()
}

func (bot MyteamBot) sendSubscribers(ctx context.Context, user string) error {
	subscribers, err := bot.subscr.Subscribers(ctx)
	if err != nil {
		return err
	}
	text := strings.Builder{}
	for _, subscr := range subscribers {
		text.WriteString("@[")
		text.WriteString(subscr.User)
		text.WriteString("] ")
		if subscr.WO {
			text.WriteString("edit")
		} else {
			text.WriteString("view")
		}
		text.WriteString("\n")
	}
	message := bot.NewTextMessage(user, text.String())
	return message.Send()
}

func (bot MyteamBot) Notify(ctx context.Context, user, link, text string, _ onlineconfbot.Notification) error {
	var keyboard botgolang.Keyboard
	if link != "" {
		keyboard = botgolang.Keyboard{Rows: [][]botgolang.Button{{{Text: "Open", URL: link}}}}
	}

	message := bot.NewInlineKeyboardMessage(user, text, keyboard)
	return message.Send()
}

func (bot MyteamBot) MentionLink(user string) string {
	return "@[" + user + "]"
}

func (bot MyteamBot) ParamLink(param, link string) string {
	return param
}

func (bot MyteamBot) ID() int                 { return bot.id }
func (bot MyteamBot) FilterSubscribers() bool { return true }
