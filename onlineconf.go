package onlineconfbot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

var ErrStatusNotOK = errors.New("response status code is not 200")

type NotificationsResponse struct {
	Notifications []Notification `json:"notifications"`
	LastID        int            `json:"lastID"`
}

func getNotifications(ctx context.Context, lastID, limit int) (*NotificationsResponse, error) {
	uri, err := url.ParseRequestURI(config.GetString("/onlineconf/botapi/url", ""))
	if err != nil {
		return nil, err
	}
	uri.Path = "/botapi/notification/"
	uri.RawQuery = url.Values{
		"lastID": []string{strconv.Itoa(lastID)},
		"limit":  []string{strconv.Itoa(limit)},
		"wait":   []string{config.GetString("/onlineconf/botapi/wait", "60")},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", uri.String(), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(
		config.GetString("/onlineconf/botapi/username", commandName),
		config.GetString("/onlineconf/botapi/password", ""),
	)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Ctx(ctx).Error().Str("method", req.Method).Str("url", req.URL.String()).Err(err).Msg("failed to get notifications")
		}
		return nil, err
	}
	if resp.StatusCode != 200 {
		log.Ctx(ctx).Error().Str("method", req.Method).Str("url", req.URL.String()).Int("status", resp.StatusCode).Msg("failed to get notifications")
		return nil, ErrStatusNotOK
	}
	var response NotificationsResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func processNotifications(ctx context.Context, bot Bot, limit int) (next bool, err error) {
	waitCtx, cancel := context.WithCancel(log.Ctx(ctx).WithContext(context.Background()))
	defer cancel()
	go func() {
		select {
		case <-ctx.Done():
			timer := time.NewTimer(time.Duration(config.GetInt("/stop-timeout", 15)) * time.Second)
			select {
			case <-timer.C:
				log.Ctx(ctx).Warn().Msg("timeout, not all notifications was sent")
				cancel()
			case <-waitCtx.Done():
				timer.Stop()
			}
		case <-waitCtx.Done():
		}
	}()

	tx, err := db.BeginTx(waitCtx)
	if err != nil {
		return false, err
	}
	lastID, err := tx.GetLastID(ctx, bot.ID())
	if err != nil {
		return false, err
	}
	log.Ctx(ctx).Debug().Int("lastID", lastID).Msg("got lastID")
	defer tx.Commit()

	if lastID == 0 {
		limit = 0
	}
	notifications, err := getNotifications(ctx, lastID, limit)
	if err != nil {
		return false, err
	}
	if notifications.LastID == lastID && len(notifications.Notifications) == 0 {
		return false, nil
	}

	newLastID := 0
	defer func() {
		if newLastID != 0 {
			if setErr := tx.SetLastID(waitCtx, newLastID, bot.ID()); setErr == nil {
				log.Ctx(ctx).Debug().Int("lastID", newLastID).Msg("new lastID")
			} else if err == nil || errors.Is(err, context.Canceled) {
				err = setErr
			}
		}
	}()

	notifier := newNotifier(bot)
	for _, notification := range notifications.Notifications {
		err := notifier.notify(waitCtx, notification)
		if err != nil {
			return false, err
		}
		newLastID = notification.ID
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}
	}
	newLastID = notifications.LastID
	return len(notifications.Notifications) > 0, nil
}

func notificationsReceiver(ctx context.Context, bot Bot) {
	for {
		timer := time.NewTimer(time.Duration(config.GetInt("/onlineconf/interval", 1)) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			for processNow := true; processNow; {
				var err error
				processNow, err = processNotifications(ctx, bot, config.GetInt("/onlineconf/batch-size", 100))
				if err != nil && !errors.Is(err, context.Canceled) {
					log.Ctx(ctx).Error().Err(err).Msg("failed to process notifications")
				}
			}
		}
	}
}

type Notifier struct {
	bot     Bot
	userMap map[string]string
	domain  string
}

func newNotifier(bot Bot) Notifier {
	ret := Notifier{
		bot:    bot,
		domain: config.GetString("/user/domain", ""),
	}

	config.GetStruct("/user/map", &ret.userMap)
	return ret
}

func (ntf *Notifier) mapUser(origUser string) string {
	user, ok := ntf.userMap[origUser]
	if !ok {
		user = origUser
	}

	if ntf.domain != "" && !strings.Contains(user, "@") {
		user += "@" + ntf.domain
	}

	return user
}

func (ntf *Notifier) notify(ctx context.Context, notification Notification) error {
	users := make(map[string]string, len(notification.Users))

	for user, access := range notification.Users {
		users[ntf.mapUser(user)] = access
	}

	if len(users) == 0 {
		return nil
	}

	notification.mappedAuthor = ntf.bot.MentionLink(ntf.mapUser(notification.Author))

	var err error
	notifyUsers := []string{}
	if ntf.bot.FilterSubscribers() {
		notifyUsers, err = db.FilterSubscribed(ctx, users)
		if err != nil {
			return err
		}
	} else {
		notifyUsers = append(notifyUsers, notification.Author)
	}

	link := ""
	if linkURLstr, hasLinkURL := config.GetStringIfExists("/onlineconf/link-url"); hasLinkURL {
		if linkURL, err := url.ParseRequestURI(linkURLstr); err == nil {
			linkURL.Fragment = notification.Path
			link = linkURL.String()
		} else {
			log.Ctx(ctx).Warn().Err(err).Msg("failed to parse link URL")
		}
	}

	notification.Path = ntf.bot.ParamLink(notification.Path, link)

	text := notification.Text()

	for _, user := range notifyUsers {
		if err = ntf.bot.Notify(ctx, user, link, text, notification); err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("failed to send notification")
		}
	}

	return nil
}
