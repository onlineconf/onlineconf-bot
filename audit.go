package onlineconfbot

import (
	"context"
	"strings"
)

// AuditEvent is one row of the OnlineConf audit log as served by the BotAPI
// audit feed (`GET /botapi/audit/`, scope `audit`): who did what to which
// parameter — today the reveals of encrypted values. It never carries a
// value; it is delivered to the subscribed users who have access to the
// parameter — its owners.
type AuditEvent struct {
	ID           int               `json:"id"`
	Path         string            `json:"path"`
	User         string            `json:"user"`
	mappedUser   string            `json:"-"`    // user's messenger account
	Kind         string            `json:"kind"` // reveal_view, reveal_history, or a kind of a newer admin
	MTime        string            `json:"mtime"`
	Notification string            `json:"notification"`
	Users        map[string]string `json:"users"`
}

type AuditEventsResponse struct {
	Events []AuditEvent `json:"events"`
	LastID int          `json:"lastID"`
}

func (event *AuditEvent) entryID() int {
	return event.ID
}

func (event *AuditEvent) deliver(ctx context.Context, ntf *Notifier) error {
	e := *event
	e.mappedUser = ntf.bot.MentionLink(ntf.mapUser(e.User))
	var link string
	e.Path, link = ntf.paramLink(ctx, e.Path)
	return ntf.send(ctx, e.Users, link, e.Text())
}

// Text renders the event. A kind this bot does not know — from a newer
// admin — is still delivered, named as it came.
func (event *AuditEvent) Text() string {
	text := strings.Builder{}
	text.WriteString(event.MTime)
	text.WriteString("\n")
	text.WriteString(avatar(event.User))
	text.WriteString(" ")
	text.WriteString(event.mappedUser)
	text.WriteString("\n")
	switch event.Kind {
	case "reveal_view":
		text.WriteString("🔓 revealed ")
	case "reveal_history":
		text.WriteString("🔓 revealed the history of ")
	default:
		text.WriteString(event.Kind)
		text.WriteString(" on ")
	}
	text.WriteString(event.Path)
	return text.String()
}
