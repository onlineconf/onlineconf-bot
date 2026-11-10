package onlineconfbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

var ErrStatusNotOK = errors.New("response status code is not 200")

// errFeedUnavailable marks a feed the admin does not serve (an older
// onlineconf-admin without it, or a bot account lacking its scope): the
// receiver backs off instead of hammering the endpoint.
var errFeedUnavailable = errors.New("feed unavailable")

// feedEntry is one event of a BotAPI feed.
type feedEntry interface {
	entryID() int
	deliver(ctx context.Context, ntf *Notifier) error
}

// Cursor rows of the feeds in the `lastid` table. The change notifications
// keep the historical row 0, and the small ids next to it are left free (a
// deployment may run one bot per messenger, each with a row of its own), so
// the audit feed stores its cursor well apart, in row 1000.
const (
	notificationCursorID = 0
	auditCursorID        = 1000
)

// feed is a long-polled, cursor-based BotAPI event stream. Two exist: the
// change notifications, and the audit events (the reveals of encrypted
// values). Each keeps its own cursor row in the `lastid` table.
type feed struct {
	name     string
	path     string
	cursorID int
	decode   func(r io.Reader) (entries []feedEntry, lastID int, err error)
}

var notificationFeed = feed{
	name:     "notifications",
	path:     "/botapi/notification/",
	cursorID: notificationCursorID,
	decode: func(r io.Reader) ([]feedEntry, int, error) {
		var response NotificationsResponse
		if err := json.NewDecoder(r).Decode(&response); err != nil {
			return nil, 0, err
		}
		entries := make([]feedEntry, 0, len(response.Notifications))
		for i := range response.Notifications {
			entries = append(entries, &response.Notifications[i])
		}
		return entries, response.LastID, nil
	},
}

var auditFeed = feed{
	name:     "audit events",
	path:     "/botapi/audit/",
	cursorID: auditCursorID,
	decode: func(r io.Reader) ([]feedEntry, int, error) {
		var response AuditEventsResponse
		if err := json.NewDecoder(r).Decode(&response); err != nil {
			return nil, 0, err
		}
		entries := make([]feedEntry, 0, len(response.Events))
		for i := range response.Events {
			entries = append(entries, &response.Events[i])
		}
		return entries, response.LastID, nil
	},
}

type NotificationsResponse struct {
	Notifications []Notification `json:"notifications"`
	LastID        int            `json:"lastID"`
}

func (f feed) get(ctx context.Context, lastID, limit int) ([]feedEntry, int, error) {
	uri, err := url.ParseRequestURI(config.GetString("/onlineconf/botapi/url", ""))
	if err != nil {
		return nil, 0, err
	}
	uri.Path = f.path
	uri.RawQuery = url.Values{
		"lastID": []string{strconv.Itoa(lastID)},
		"limit":  []string{strconv.Itoa(limit)},
		"wait":   []string{config.GetString("/onlineconf/botapi/wait", "60")},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", uri.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	req.SetBasicAuth(
		config.GetString("/onlineconf/botapi/username", commandName),
		config.GetString("/onlineconf/botapi/password", ""),
	)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			log.Ctx(ctx).Error().Str("method", req.Method).Str("url", req.URL.String()).Err(err).Msgf("failed to get %s", f.name)
		}
		return nil, 0, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return f.decode(resp.Body)
	case http.StatusForbidden, http.StatusNotFound:
		return nil, 0, fmt.Errorf("%w: %s %s: %s", errFeedUnavailable, req.Method, req.URL.String(), resp.Status)
	default:
		log.Ctx(ctx).Error().Str("method", req.Method).Str("url", req.URL.String()).Int("status", resp.StatusCode).Msgf("failed to get %s", f.name)
		return nil, 0, ErrStatusNotOK
	}
}

func (f feed) process(ctx context.Context, bot Bot, limit int) (next bool, err error) {
	waitCtx, cancel := context.WithCancel(log.Ctx(ctx).WithContext(context.Background()))
	defer cancel()
	go func() {
		select {
		case <-ctx.Done():
			timer := time.NewTimer(time.Duration(config.GetInt("/stop-timeout", 15)) * time.Second)
			select {
			case <-timer.C:
				log.Ctx(ctx).Warn().Msgf("timeout, not all %s was sent", f.name)
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
	lastID, err := tx.GetLastID(ctx, f.cursorID)
	if err != nil {
		tx.Rollback()
		return false, err
	}
	log.Ctx(ctx).Debug().Str("feed", f.name).Int("lastID", lastID).Msg("got lastID")
	defer tx.Commit()

	if lastID == 0 {
		limit = 0
	}
	entries, feedLastID, err := f.get(ctx, lastID, limit)
	if err != nil {
		return false, err
	}
	if feedLastID == lastID && len(entries) == 0 {
		return false, nil
	}

	newLastID := 0
	defer func() {
		if newLastID != 0 {
			if setErr := tx.SetLastID(waitCtx, f.cursorID, newLastID); setErr == nil {
				log.Ctx(ctx).Debug().Str("feed", f.name).Int("lastID", newLastID).Msg("new lastID")
			} else if err == nil || errors.Is(err, context.Canceled) {
				err = setErr
			}
		}
	}()

	notifier := newNotifier(bot)
	for _, entry := range entries {
		err := entry.deliver(waitCtx, &notifier)
		if err != nil {
			return false, err
		}
		newLastID = entry.entryID()
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}
	}
	newLastID = feedLastID
	return len(entries) > 0, nil
}

// receiver polls the feed until the context is cancelled.
func (f feed) receiver(ctx context.Context, bot Bot) {
	unavailable := false
	for {
		interval := time.Duration(config.GetInt("/onlineconf/interval", 1)) * time.Second
		if unavailable {
			// an admin without this feed (or a bot account without its
			// scope): check again after a long-poll's worth of time
			interval = time.Duration(config.GetInt("/onlineconf/botapi/wait", 60)) * time.Second
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			for processNow := true; processNow; {
				var err error
				processNow, err = f.process(ctx, bot, config.GetInt("/onlineconf/batch-size", 100))
				switch {
				case err == nil:
					unavailable = false
				case errors.Is(err, errFeedUnavailable):
					if !unavailable {
						log.Ctx(ctx).Warn().Err(err).Msgf("the %s feed is not available, will retry later", f.name)
					}
					unavailable = true
				case !errors.Is(err, context.Canceled):
					log.Ctx(ctx).Error().Err(err).Msgf("failed to process %s", f.name)
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

// paramLink renders the parameter path as a link to the OnlineConf UI, and
// returns the bare link for the messenger's use.
func (ntf *Notifier) paramLink(ctx context.Context, path string) (rendered, link string) {
	if linkURLstr, hasLinkURL := config.GetStringIfExists("/onlineconf/link-url"); hasLinkURL {
		if linkURL, err := url.ParseRequestURI(linkURLstr); err == nil {
			linkURL.Fragment = path
			link = linkURL.String()
		} else {
			log.Ctx(ctx).Warn().Err(err).Msg("failed to parse link URL")
		}
	}
	return ntf.bot.ParamLink(path, link), link
}

// send delivers a rendered message about a parameter to the subscribed users
// among those who have access to it (the `users` map of a feed entry).
func (ntf *Notifier) send(ctx context.Context, users map[string]string, link, text string) error {
	mapped := make(map[string]string, len(users))
	for user, access := range users {
		mapped[ntf.mapUser(user)] = access
	}
	if len(mapped) == 0 {
		return nil
	}

	notifyUsers, err := db.FilterSubscribed(ctx, mapped)
	if err != nil {
		return err
	}

	for _, user := range notifyUsers {
		if err = ntf.bot.Notify(ctx, user, link, text); err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("failed to send notification")
		}
	}

	return nil
}

func (ntf *Notifier) notify(ctx context.Context, notification Notification) error {
	notification.mappedAuthor = ntf.bot.MentionLink(ntf.mapUser(notification.Author))
	var link string
	notification.Path, link = ntf.paramLink(ctx, notification.Path)
	return ntf.send(ctx, notification.Users, link, notification.Text())
}
