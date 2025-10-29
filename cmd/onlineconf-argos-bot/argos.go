package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"
	onlineconfbot "github.com/onlineconf/onlineconf-bot"
	"github.com/onlineconf/onlineconf-go"
	"github.com/rs/zerolog/log"
)

type ArgosBot struct {
	subscr     onlineconfbot.SubscriptionStorage
	host       string
	port       int
	protocol   string
	metricName string
	fields     map[string]string
	sentryDSN  string
	id         int
}

type metric struct {
	Name    string                 `json:"name"`
	Tags    map[string]interface{} `json:"tags"`
	Counter int                    `json:"counter"`
}

type argosMessage struct {
	Metrics []metric `json:"metrics"`
}

type argosNotification struct {
	onlineconfbot.Notification
	TaskName string
}

var taskNameRgx = regexp.MustCompile(`([A-Za-z]+\-\d+)`)
var _ onlineconfbot.Bot = ArgosBot{}

func NewArgosBot(config *onlineconf.Module, subscr onlineconfbot.SubscriptionStorage) (ArgosBot, error) {
	host := config.GetString("/argos/host", "")
	if host == "" {
		return ArgosBot{}, fmt.Errorf("empty host collector")
	}
	protocol := config.GetString("/argos/protocol", "udp")
	if protocol == "" {
		return ArgosBot{}, fmt.Errorf("empty protocol collector")
	}
	port := config.GetInt("/argos/port", 0)
	if port == 0 {
		return ArgosBot{}, fmt.Errorf("empty port collector")
	}
	fields := map[string]string{}
	config.GetStruct("/argos/fields", &fields)
	if len(fields) == 0 {
		return ArgosBot{}, fmt.Errorf("empty fields definition")
	}
	metricName := config.GetString("/argos/metric_name", "")
	if metricName == "" {
		return ArgosBot{}, fmt.Errorf("empty metric_name")
	}
	sentryDSN := config.GetString("/argos/sentry_dsn", "")
	id := config.GetInt("/argos/id", 0)
	return ArgosBot{subscr, host, port, protocol, metricName, fields, sentryDSN, id}, nil
}

func (bot ArgosBot) UpdatesProcessor(ctx context.Context) {}

func (bot ArgosBot) Notify(ctx context.Context, user, link, text string, notification onlineconfbot.Notification) error {
	addr := fmt.Sprintf("%s:%d", bot.host, bot.port)
	conn, err := net.Dial(bot.protocol, addr)
	if err != nil {
		return fmt.Errorf("error establishing connection: %v", err)
	}
	defer conn.Close()

	argosNotify := argosNotification{Notification: notification}

	match := taskNameRgx.FindStringSubmatch(argosNotify.Comment)
	if len(match) > 0 {
		argosNotify.TaskName = match[0]
	}

	defer sentry.Flush(2 * time.Second)
	if dsn := bot.sentryDSN; dsn != "" {
		err := sentry.Init(sentry.ClientOptions{
			Dsn: dsn,
		})
		if err != nil {
			return fmt.Errorf("sentry initialization failed: %v", err)
		}
	}

	val := reflect.ValueOf(argosNotify)
	data := map[string]interface{}{}
	for k, v := range bot.fields {
		field := val.FieldByName(v)
		if !field.IsValid() || !field.CanInterface() {
			data[k] = v
			continue
		}
		switch field.Interface().(type) {
		case string:
			data[k] = field.Interface().(string)
		case int:
			data[k] = strconv.Itoa(field.Interface().(int))
		}
	}

	argosMsg := argosMessage{
		Metrics: []metric{
			{
				Name:    bot.metricName,
				Tags:    data,
				Counter: 1,
			},
		},
	}
	message, err := json.Marshal(argosMsg)
	if err != nil {
		return fmt.Errorf("prepare json message failure: %v", err)
	}
	log.Ctx(ctx).Debug().Msg(fmt.Sprintf("argos notification: %+v", string(message)))
	if argosNotify.TaskName == "" {
		sentry.CaptureException(fmt.Errorf("empty taskName, message: %s", string(message)))
	}

	_, err = conn.Write(message)
	if err != nil {
		return fmt.Errorf("error sending message: %v\n", err)
	}

	return nil
}

func (bot ArgosBot) MentionLink(user string) string { return user }

func (bot ArgosBot) ParamLink(param, link string) string { return param }

func (bot ArgosBot) ID() int                 { return bot.id }
func (bot ArgosBot) FilterSubscribers() bool { return false }
