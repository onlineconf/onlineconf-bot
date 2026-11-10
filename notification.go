package onlineconfbot

import (
	"context"
	"encoding/json"
	"hash/crc32"
	"strings"
)

type Notification struct {
	ID           int               `json:"id"`
	Path         string            `json:"path"`
	Version      int               `json:"version"`
	ContentType  string            `json:"type"`
	Encrypted    bool              `json:"encrypted"` // the value is a secret: served as null, shown as a lock
	Value        NullString        `json:"value"`
	MTime        string            `json:"mtime"`
	Author       string            `json:"author"`
	mappedAuthor string            `json:"-"` // author's messenger account
	Comment      string            `json:"comment"`
	Action       string            `json:"action"`
	Notification string            `json:"notification"`
	Users        map[string]string `json:"users"`
}

func (notification *Notification) entryID() int {
	return notification.ID
}

func (notification *Notification) deliver(ctx context.Context, ntf *Notifier) error {
	return ntf.notify(ctx, *notification)
}

// caseBranch is one entry of a case value: a condition (at most one of the
// four), the branch's type and value, and whether the value is a secret.
type caseBranch struct {
	Server     *string `json:"server"`
	Group      *string `json:"group"`
	Datacenter *string `json:"datacenter"`
	Service    *string `json:"service"`
	Mime       string  `json:"mime"`
	Value      *string `json:"value"`
	Encrypted  bool    `json:"encrypted"`
}

func (notification *Notification) Text() string {
	text := strings.Builder{}
	text.WriteString(notification.MTime)
	text.WriteString("\n")
	text.WriteString(avatar(notification.Author))
	text.WriteString(" ")
	text.WriteString(notification.mappedAuthor)
	text.WriteString("\n")

	switch notification.Action {
	case "delete":
		text.WriteString("❌️")
	case "create":
		text.WriteString("🆕️")
	case "modify":
		text.WriteString("✏️")
	}
	text.WriteString(" ")
	text.WriteString(notification.Path)
	if notification.Action != "delete" && notification.Notification == "with-value" {
		if notification.Encrypted {
			text.WriteString(" 🔒")
		}
		if ct := contentTypeSymbol(notification.ContentType); ct != "" {
			text.WriteString(" ")
			text.WriteString(ct)
		}
		if notification.Value.Valid && !notification.Encrypted {
			switch notification.ContentType {
			case "application/x-case":
				var data []caseBranch
				err := json.Unmarshal([]byte(notification.Value.String), &data)
				if err == nil {
					for _, c := range data {
						text.WriteString("\n")
						switch {
						case c.Server != nil:
							text.WriteString("ⓗ ")
							text.WriteString(*c.Server)
						case c.Group != nil:
							text.WriteString("ⓖ ")
							text.WriteString(*c.Group)
						case c.Datacenter != nil:
							text.WriteString("ⓓ ")
							text.WriteString(*c.Datacenter)
						case c.Service != nil:
							text.WriteString("ⓢ ")
							text.WriteString(*c.Service)
						default:
							text.WriteString("☆️")
						}
						text.WriteString(": ")
						ct := contentTypeSymbol(c.Mime)
						text.WriteString(ct)
						switch {
						case c.Encrypted:
							if ct != "" {
								text.WriteString(" ")
							}
							text.WriteString("🔒")
						case c.Value != nil:
							if ct != "" {
								text.WriteString(" ")
							}
							if strings.ContainsRune(*c.Value, '"') {
								text.WriteString("«")
								text.WriteString(*c.Value)
								text.WriteString("»")
							} else {
								text.WriteString("\"")
								text.WriteString(*c.Value)
								text.WriteString("\"")
							}
						}
					}
				} else {
					blockQuote(&text, notification.Value.String, "")
				}
			case "application/x-symlink":
				text.WriteString("\n")
				text.WriteString(notification.Value.String)
			default:
				blockQuote(&text, notification.Value.String, notification.ContentType)
			}
		}
	}
	if notification.Comment != "" {
		text.WriteString("\n🗒 ")
		text.WriteString(notification.Comment)
	}
	return text.String()
}

func blockQuote(text *strings.Builder, s, ctype string) {
	text.WriteString("\n")

	if s != "" {
		switch ctype {
		case "application/json":
			ctype = "json\n"
		case "application/x-yaml":
			ctype = "yaml\n"
		default:
			ctype = "\n"
		}

		text.WriteString("```")
		text.WriteString(ctype)
		text.WriteString(s)

		if strings.HasSuffix(s, "\n") {
			text.WriteString("```")
		} else {
			text.WriteString("\n```")
		}
	}
}

var avatars = []rune("🐀🐁🐂🐃🐄🐅🐆🐇🐈🐉🐊🐋🐌🐍🐎🐏🐐🐑🐒🐓🐕🐖🐗🐘🐙🐛🐜🐝🐞🐟🐠🐡🐢🐥🐨🐩🐪🐫🐬🐭🐮🐯🐰🐱🐲🐳🐴🐵🐶🐷🐸🐹🐺🐻🐼" +
	"🐿🦀🦁🦂🦃🦄🦅🦆🦇🦈🦉🦊🦋🦌🦍🦎🦏🦐🦑🦒🦓🦔🦕🦖🦗🦘🦙🦚🦛🦜🦝🦞🦟🦠🦡🦢🦥🦦🦧🦨🦩")

func avatar(user string) string {
	return string(avatars[int(crc32.ChecksumIEEE([]byte(user)))%len(avatars)])
}

func contentTypeSymbol(contentType string) string {
	switch contentType {
	case "application/x-null":
		return "∅"
	case "application/x-symlink":
		return "➦"
	case "application/x-case":
		return "⌥"
	case "application/x-template":
		return "✄"
	case "application/json":
		return "🄹"
	case "application/x-yaml":
		return "🅈"
	default:
		return ""
	}
}
