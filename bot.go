package onlineconfbot

import (
	"context"
	"fmt"
)

type Bot interface {
	UpdatesProcessor(context.Context)
	Notify(ctx context.Context, user, link, text string, notification Notification) error
	MentionLink(string) string
	ParamLink(param, link string) string
	ID() int
	FilterSubscribers() bool
}

type debugBot struct{}

var _ Bot = debugBot{}

func (debugBot) UpdatesProcessor(context.Context) {
}

func (debugBot) Notify(_ context.Context, user, link, text string, _ Notification) error {
	_, err := fmt.Printf("to: %s\n%s\n%s\n", user, link, text)
	return err
}

func (debugBot) MentionLink(user string) string {
	return user
}

func (debugBot) ParamLink(param, link string) string {
	return param
}

func IsAdmin(user string) bool {
	for _, u := range config.GetStrings("/user/admins", nil) {
		if u == user {
			return true
		}
	}
	return false
}

func (debugBot) ID() int                 { return 0 }
func (debugBot) FilterSubscribers() bool { return true }
