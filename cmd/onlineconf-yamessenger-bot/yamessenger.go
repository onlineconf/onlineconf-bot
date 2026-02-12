package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	onlineconfbot "github.com/onlineconf/onlineconf-bot"
	"github.com/onlineconf/onlineconf-go"
	"github.com/rs/zerolog/log"
)

type YaMessengerBot struct {
	apiURL string
	token  string
	subscr onlineconfbot.SubscriptionStorage
	client *http.Client
}

var _ onlineconfbot.Bot = &YaMessengerBot{}

func NewYaMessengerBot(config *onlineconf.Module, subscr onlineconfbot.SubscriptionStorage) (*YaMessengerBot, error) {
	apiURL := config.GetString("/yamessenger/api-url", "https://botapi.messenger.yandex.net")
	token := config.GetString("/yamessenger/token", "")
	if token == "" {
		return nil, errors.New("please specify the Yandex Messenger bot token using /yamessenger/token")
	}

	return &YaMessengerBot{
		apiURL: strings.TrimRight(apiURL, "/"),
		token:  token,
		subscr: subscr,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Yandex Messenger Bot API types

type yaUpdate struct {
	UpdateID  int      `json:"update_id"`
	MessageID int64    `json:"message_id"`
	Timestamp int64    `json:"timestamp"`
	From      yaSender `json:"from"`
	Chat      yaChat   `json:"chat"`
	Text      string   `json:"text"`
}

type yaSender struct {
	Login       string `json:"login"`
	DisplayName string `json:"display_name"`
	Robot       bool   `json:"robot"`
}

type yaChat struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type yaGetUpdatesResponse struct {
	OK      bool       `json:"ok"`
	Updates []yaUpdate `json:"updates"`
}

type yaSendTextRequest struct {
	Login          string     `json:"login,omitempty"`
	ChatID         string     `json:"chat_id,omitempty"`
	Text           string     `json:"text"`
	InlineKeyboard []yaButton `json:"inline_keyboard,omitempty"`
}

type yaButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}

type yaSendTextResponse struct {
	OK          bool   `json:"ok"`
	MessageID   int64  `json:"message_id,omitempty"`
	Description string `json:"description,omitempty"`
}

func (bot *YaMessengerBot) UpdatesProcessor(ctx context.Context) {
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		updates, err := bot.getUpdates(ctx, offset)
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("failed to get updates")
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		for _, update := range updates {
			if update.From.Robot {
				continue
			}

			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}

			bot.handleUpdate(ctx, update)
		}

		if len(updates) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}

func (bot *YaMessengerBot) handleUpdate(ctx context.Context, update yaUpdate) {
	text := strings.TrimSpace(update.Text)
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return
	}

	cmd := fields[0]
	args := fields[1:]
	user := update.From.Login

	var err error
	switch cmd {
	case "/start":
		err = bot.sendSubscribePrompt(user)
	case "/subscribe":
		err = bot.handleSubscribe(ctx, user, args)
	case "/stop":
		err = bot.unsubscribe(ctx, user)
	case "/subscribers":
		if onlineconfbot.IsAdmin(user) {
			err = bot.sendSubscribers(ctx, user)
		}
	case "/help":
		err = bot.sendHelp(user)
	default:
		err = bot.sendText(ctx, user, "Unknown command. Use /help to see available commands.")
	}

	if err != nil {
		log.Ctx(ctx).Error().Err(err).Str("command", cmd).Str("user", user).Msg("failed to handle command")
	}
}

func (bot *YaMessengerBot) sendSubscribePrompt(user string) error {
	req := yaSendTextRequest{
		Login: user,
		Text:  "Choose parameters you want to subscribe to:\n/subscribe edit - parameters you can edit\n/subscribe view - parameters you can view",
	}
	return bot.doSendText(context.Background(), req)
}

func (bot *YaMessengerBot) handleSubscribe(ctx context.Context, user string, args []string) error {
	if len(args) != 1 {
		return bot.sendText(ctx, user, "Usage: /subscribe [edit|view]")
	}

	var wo bool
	switch args[0] {
	case "edit":
		wo = true
	case "view":
		wo = false
	default:
		return bot.sendText(ctx, user, "Usage: /subscribe [edit|view]")
	}

	if err := bot.subscr.Subscribe(ctx, user, wo); err != nil {
		return err
	}
	return bot.sendText(ctx, user, "You subscribed to parameters you can "+args[0])
}

func (bot *YaMessengerBot) unsubscribe(ctx context.Context, user string) error {
	if err := bot.subscr.Unsubscribe(ctx, user); err != nil {
		return err
	}
	return bot.sendText(ctx, user, "You unsubscribed")
}

func (bot *YaMessengerBot) sendSubscribers(ctx context.Context, user string) error {
	subscribers, err := bot.subscr.Subscribers(ctx)
	if err != nil {
		return err
	}

	text := strings.Builder{}
	text.WriteString("Subscribers:\n")
	for _, subscr := range subscribers {
		text.WriteString(subscr.User)
		text.WriteString(" - ")
		if subscr.WO {
			text.WriteString("edit")
		} else {
			text.WriteString("view")
		}
		text.WriteString("\n")
	}
	return bot.sendText(ctx, user, text.String())
}

func (bot *YaMessengerBot) sendHelp(user string) error {
	text := "Available commands:\n" +
		"/start - Show subscribe prompt\n" +
		"/subscribe [edit|view] - Subscribe to notifications\n" +
		"/stop - Unsubscribe from notifications\n" +
		"/subscribers - Show subscribed users (admin only)\n" +
		"/help - Show this help"
	req := yaSendTextRequest{
		Login: user,
		Text:  text,
	}
	return bot.doSendText(context.Background(), req)
}

func (bot *YaMessengerBot) Notify(ctx context.Context, user, link, text string) error {
	return bot.sendText(ctx, user, text)
}

func (bot *YaMessengerBot) MentionLink(user string) string {
	return "@" + user
}

func (bot *YaMessengerBot) ParamLink(param, link string) string {
	return param
}

// HTTP helpers

func (bot *YaMessengerBot) sendText(ctx context.Context, login, text string) error {
	return bot.doSendText(ctx, yaSendTextRequest{Login: login, Text: text})
}

func (bot *YaMessengerBot) doSendText(ctx context.Context, req yaSendTextRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal sendText request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, bot.apiURL+"/bot/v1/messages/sendText/", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create sendText request: %w", err)
	}
	httpReq.Header.Set("Authorization", "OAuth "+bot.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := bot.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sendText request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read sendText response: %w", err)
	}

	var result yaSendTextResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("unmarshal sendText response: %w", err)
	}

	if !result.OK {
		return fmt.Errorf("sendText failed: %s", result.Description)
	}

	return nil
}

func (bot *YaMessengerBot) getUpdates(ctx context.Context, offset int) ([]yaUpdate, error) {
	url := fmt.Sprintf("%s/bot/v1/messages/getUpdates/?offset=%d&limit=100", bot.apiURL, offset)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create getUpdates request: %w", err)
	}
	httpReq.Header.Set("Authorization", "OAuth "+bot.token)

	resp, err := bot.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("getUpdates request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read getUpdates response: %w", err)
	}

	var result yaGetUpdatesResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal getUpdates response: %w", err)
	}

	if !result.OK {
		return nil, fmt.Errorf("getUpdates failed")
	}

	return result.Updates, nil
}
