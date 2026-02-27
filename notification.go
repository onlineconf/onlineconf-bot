package onlineconfbot

import (
	"encoding/json"
	"hash/crc32"
	"strings"
)

type Notification struct {
	ID           int               `json:"id"`
	Path         string            `json:"path"`
	Version      int               `json:"version"`
	ContentType  string            `json:"type"`
	Value        NullString        `json:"value"`
	MTime        string            `json:"mtime"`
	Author       string            `json:"author"`
	mappedAuthor string            `json:"-"` // author's messenger account
	Comment      string            `json:"comment"`
	Action       string            `json:"action"`
	Notification string            `json:"notification"`
	Users        map[string]string `json:"users"`
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
		if ct := contentTypeSymbol(notification.ContentType); ct != "" {
			text.WriteString(" ")
			text.WriteString(ct)
		}
		if notification.Value.Valid {
			switch notification.ContentType {
			case "application/x-case":
				var data []map[string]string
				err := json.Unmarshal([]byte(notification.Value.String), &data)
				if err == nil {
					for _, c := range data {
						text.WriteString("\n")
						if s, ok := c["server"]; ok {
							text.WriteString("ⓗ ")
							text.WriteString(s)
						} else if g, ok := c["group"]; ok {
							text.WriteString("ⓖ ")
							text.WriteString(g)
						} else if d, ok := c["datacenter"]; ok {
							text.WriteString("ⓓ ")
							text.WriteString(d)
						} else if s, ok := c["service"]; ok {
							text.WriteString("ⓢ ")
							text.WriteString(s)
						} else {
							text.WriteString("☆️")
						}
						text.WriteString(": ")
						ct := contentTypeSymbol(c["mime"])
						value, ok := c["value"]
						text.WriteString(ct)
						if ok {
							if ct != "" {
								text.WriteString(" ")
							}
							if strings.ContainsRune(value, '"') {
								text.WriteString("«")
								text.WriteString(value)
								text.WriteString("»")
							} else {
								text.WriteString("\"")
								text.WriteString(value)
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
