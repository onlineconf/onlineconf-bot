package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	mm "github.com/mattermost/mattermost-server/v6/model"
	onlineconfbot "github.com/onlineconf/onlineconf-bot"
	"github.com/onlineconf/onlineconf-go"
	"github.com/rs/zerolog/log"
)

type MattermostBot struct {
	api      *mm.Client4
	ws       *mm.WebSocketClient
	commands []mmCommandHandler
	botID    string
	subscr   onlineconfbot.SubscriptionStorage
	id       int
}

var _ onlineconfbot.Bot = &MattermostBot{}

type mmCommandHandler struct {
	cmd   string
	args  string
	descr string
	color string
	// rootID is optional, userName should be taken from the GetUser API response. never use userName from message metadata.
	handler   func(mmb *MattermostBot, ctx context.Context, channelID, rootID, userID, userName string, args ...string) error
	isAllowed func(userName string) bool // nil - command is allowed for everyone
}

var mmCommands = []mmCommandHandler{
	{
		cmd:     "help",
		descr:   "Show available commands",
		color:   "#008000",
		handler: (*MattermostBot).sendHelp,
	},
	{
		cmd:     "subscribe",
		args:    "[`view`|`edit`]",
		descr:   "Subscribe to parameter change notifications for parameters you can view/edit",
		color:   "#f5b642",
		handler: (*MattermostBot).subscribe,
	},
	{
		cmd:     "unsubscribe",
		descr:   "Unsubscribe from notifications",
		color:   "#db0707",
		handler: (*MattermostBot).unsubscribe,
	},
	{
		cmd:       "subscribers",
		descr:     "Show subscribed users",
		handler:   (*MattermostBot).listSubscribers,
		isAllowed: onlineconfbot.IsAdmin,
	},
}

var mmCommandsByName = func() map[string]mmCommandHandler {
	ret := make(map[string]mmCommandHandler, len(mmCommands))

	for _, cmd := range mmCommands {
		ret[cmd.cmd] = cmd
	}

	return ret
}()

func NewMattermostBot(config *onlineconf.Module, subscr onlineconfbot.SubscriptionStorage) (*MattermostBot, error) {
	apiURL := config.GetString("/mattermost/api-url", os.Getenv("ONLINECONF_MATTERMOST_API_URL"))
	if apiURL == "" {
		return nil, errors.New("please specify the mattermost API URL using /mattermost/api-url or $ONLINECONF_MATTERMOST_API_URL")
	}

	wsURL := config.GetString("/mattermost/ws-url", os.Getenv("ONLINECONF_MATTERMOST_WS_URL"))
	if wsURL == "" {
		return nil, errors.New("please specify the mattermost Websocket URL using /mattermost/ws-url or $ONLINECONF_MATTERMOST_WS_URL")
	}

	token := config.GetString("/mattermost/token", os.Getenv("ONLINECONF_MATTERMOST_TOKEN"))
	if token == "" {
		return nil, errors.New("please specify the mattermost API token using /mattermost/token or $ONLINECONF_MATTERMOST_TOKEN")
	}

	api := mm.NewAPIv4Client(apiURL)
	api.SetToken(token)

	me, _, err := api.GetMe("")
	if err != nil {
		return nil, err
	}

	ws, err := mm.NewWebSocketClient4(wsURL, token)
	if err != nil {
		return nil, err
	}
	id := config.GetInt("/mattermost/id", 0)
	return &MattermostBot{
		api:      api,
		ws:       ws,
		commands: mmCommands,
		botID:    me.Id,
		subscr:   subscr,
		id:       id,
	}, nil
}

func (mmb *MattermostBot) UpdatesProcessor(ctx context.Context) {
	mmb.ws.Listen()

	go func() {
		<-ctx.Done()
		mmb.ws.Close()
	}()

	go func() {
		for isTimeout := range mmb.ws.PingTimeoutChannel {
			if isTimeout {
				log.Ctx(ctx).Error().Msg("websocket timeout")
			}
		}

		log.Ctx(ctx).Info().Msg("websocket PingTimeoutChannel is closed")
	}()

	go func() {
		for resp := range mmb.ws.ResponseChannel {
			log.Ctx(ctx).Info().Interface("resp", resp).Send()
		}

		log.Ctx(ctx).Info().Msg("websocket ResponseChannel is closed")
	}()

	for event := range mmb.ws.EventChannel {
		log.Ctx(ctx).Trace().Str("event_type", event.EventType()).Interface("data", event.GetData()).Send()

		switch event.EventType() {
		case mm.WebsocketEventHello:
			if version, err := mmGetString(event, "server_version"); err == nil {
				log.Ctx(ctx).Info().Str("server_version", version).Msg("connected to websocket endpoint")
			}

		case mm.WebsocketEventPosted:
			if err := mmb.handlePost(ctx, event); err != nil {
				log.Ctx(ctx).Err(err).Msg("WebsocketEventPosted processing failed")
			}

		case mm.WebsocketEventDirectAdded:
			userID, err := mmGetString(event, "creator_id")
			if err != nil {
				log.Ctx(ctx).Err(err).Msg("WebsocketEventDirectAdded: mmGetString(creator_id)")
				break
			}

			user, _, err := mmb.api.GetUser(userID, "")
			if err != nil {
				log.Ctx(ctx).Err(err).Str("user_id", userID).Msg("WebsocketEventDirectAdded: GetUser failed")
				break
			}

			ch, _, err := mmb.api.CreateDirectChannel(userID, mmb.botID)
			if err != nil {
				log.Ctx(ctx).Err(err).Str("user_id", userID).Msg("WebsocketEventDirectAdded: CreateDirectChannel failed")
			}

			mmb.sendHelp(ctx, ch.Id, "", userID, user.Username)
		}
	}

	log.Ctx(ctx).Info().Msg("websocket EventChannel is closed")
}

func (mmb *MattermostBot) handlePost(ctx context.Context, event *mm.WebSocketEvent) error {
	in, err := unmarshalPost(event)
	if err != nil {
		return err
	}

	fromBotI := in.GetProp("from_bot")
	switch fromBot := fromBotI.(type) {
	case bool:
		if fromBot {
			return nil
		}
	case string:
		if fromBot == "true" {
			return nil
		}
	}

	user, _, err := mmb.api.GetUser(in.UserId, "")
	if err != nil {
		return err
	}

	cmd := strings.Fields(in.Message)
	if len(cmd) == 0 {
		return nil
	}

	handler, ok := mmCommandsByName[cmd[0]]
	if ok && handler.isAllowed != nil {
		ok = handler.isAllowed(user.Username)
	}

	if !ok {
		if err := mmb.send(in.ChannelId, in.RootId, in.UserId, "⚠️ Unknown command: `"+cmd[0]+"`"); err != nil {
			return err
		}

		return mmb.sendHelp(ctx, in.ChannelId, in.RootId, in.UserId, user.Username)
	}

	err = handler.handler(mmb, ctx, in.ChannelId, in.RootId, in.UserId, user.Username, cmd[1:]...)
	if err != nil {
		_ = mmb.send(in.ChannelId, in.RootId, in.UserId, "⛔ Internal error")
	}

	return err
}

func unmarshalPost(event *mm.WebSocketEvent) (*mm.Post, error) {
	post, ok := event.GetData()["post"].(string)
	if !ok {
		return nil, errors.New("post data is empty")
	}

	var ret mm.Post
	if err := json.Unmarshal([]byte(post), &ret); err != nil {
		return nil, err
	}

	return &ret, nil
}

func mmGetString(ev *mm.WebSocketEvent, k string) (string, error) {
	val, ok := ev.GetData()[k]
	if !ok {
		return "", errors.New("mmGetString: key " + strconv.Quote(k) + " not found")
	}

	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("mmGetString: key %q is %T, but string was expected", k, val)
	}

	return s, nil
}

func (mmb *MattermostBot) sendHelp(ctx context.Context, chID, rootID, userID, userName string, _ ...string) error {
	out := &mm.Post{
		ChannelId: chID,
		RootId:    rootID,
		UserId:    userID,
		Message:   "*Available commands:*",
	}

	attachments := make([]*mm.SlackAttachment, len(mmb.commands))

	for i, cmd := range mmb.commands {
		footer := ""
		if cmd.isAllowed != nil {
			footer = "admin only"
		}

		attachments[i] = &mm.SlackAttachment{
			Title:  cmd.cmd + " " + cmd.args,
			Text:   cmd.descr,
			Footer: footer,
			Color:  cmd.color,
		}
	}

	out.AddProp("attachments", attachments)

	_, _, err := mmb.api.CreatePost(out)
	return err
}

func (mmb *MattermostBot) subscribe(ctx context.Context, channelID, rootID, userID, userName string, args ...string) error {
	if len(args) != 1 {
		return mmb.send(channelID, rootID, userID, "⚠️ subscribe command takes exactly one argument ([`view`|`edit`])")
	}

	canWrite := false
	switch args[0] {
	case "view":
	case "edit":
		canWrite = true
	default:
		return mmb.send(channelID, rootID, userID, "⚠️ subscribe: invalid subscription mode (use `view` or `edit`)")
	}

	if err := mmb.subscr.Subscribe(ctx, userName, canWrite); err != nil {
		return err
	}

	return mmb.send(channelID, rootID, userID, "✅ You have subscribed to parameters you can `"+args[0]+"`")
}

func (mmb *MattermostBot) unsubscribe(ctx context.Context, channelID, rootID, userID, userName string, args ...string) error {
	err := mmb.subscr.Unsubscribe(ctx, userName)
	if err != nil {
		return err
	}

	return mmb.send(channelID, rootID, userID, "❌️ You have unsubscribed")
}

func (mmb *MattermostBot) listSubscribers(ctx context.Context, channelID, rootID, userID, userName string, args ...string) error {
	subscribers, err := mmb.subscr.Subscribers(ctx)
	if err != nil {
		return err
	}

	resp := strings.Builder{}
	resp.WriteString("| User | Mode |\n|---|---|\n")

	for _, subscr := range subscribers {
		resp.WriteString("|@")
		resp.WriteString(subscr.User)

		if subscr.WO {
			resp.WriteString("|edit|\n")
		} else {
			resp.WriteString("|view|\n")
		}
	}

	return mmb.send(channelID, rootID, userID, resp.String())
}

func (mmb *MattermostBot) send(channelID, rootID, userID, message string) error {
	_, _, err := mmb.api.CreatePost(&mm.Post{
		ChannelId: channelID,
		RootId:    rootID,
		UserId:    userID,
		Message:   message,
	})
	return err
}

func (mmb *MattermostBot) Notify(ctx context.Context, userName, link, text string, _ onlineconfbot.Notification) error {
	user, _, err := mmb.api.GetUserByUsername(userName, "")
	if err != nil {
		return err
	}

	ch, _, err := mmb.api.CreateDirectChannel(user.Id, mmb.botID)
	if err != nil {
		return err
	}

	return mmb.send(ch.Id, "", user.Id, "***\n"+text)
}

func (mmb *MattermostBot) MentionLink(user string) string {
	return "@" + user
}

var (
	// XXX backslashes inside links work in MM mobile only.
	paramRepl = strings.NewReplacer(`[`, `\[`, `]`, `\]`)
	linkRepl  = strings.NewReplacer(`(`, `\(`, `)`, `\)`)
)

func (mmb *MattermostBot) ParamLink(param, link string) string {
	return "[" + paramRepl.Replace(param) + "](" + linkRepl.Replace(link) + ")"
}

func (mmb *MattermostBot) ID() int                 { return mmb.id }
func (mmb *MattermostBot) FilterSubscribers() bool { return true }
