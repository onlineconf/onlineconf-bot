package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
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
		"wait":   []string{"60"},
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

func processNotifications(ctx context.Context, bot *myteamBot, limit int) (next bool, err error) {
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
	lastID, err := tx.GetLastID(ctx)
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
			if setErr := tx.SetLastID(waitCtx, newLastID); setErr == nil {
				log.Ctx(ctx).Debug().Int("lastID", newLastID).Msg("new lastID")
			} else if err == nil || errors.Is(err, context.Canceled) {
				err = setErr
			}
		}
	}()
	for _, notification := range notifications.Notifications {
		err := bot.notify(waitCtx, notification)
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

func notificationsReceiver(ctx context.Context, bot *myteamBot) {
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
