package core

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/golem-mc/golem/plugin"
)

//go:embed data/strings.json
var defaultStringsJSON []byte

// Messages manages FAWE's upstream lang/strings.json captions and legacy '&' color formatting.
type Messages struct {
	mu      sync.RWMutex
	strings map[string]string
}

// LoadMessages loads embedded FAWE strings.json and overlays <dataDir>/lang/strings.json if present.
func LoadMessages(dataDir string) *Messages {
	m := &Messages{strings: make(map[string]string)}
	_ = json.Unmarshal(defaultStringsJSON, &m.strings)
	if dataDir != "" {
		overridePath := filepath.Join(dataDir, "lang", "strings.json")
		if data, err := os.ReadFile(overridePath); err == nil {
			var ov map[string]string
			if json.Unmarshal(data, &ov) == nil {
				for k, v := range ov {
					m.strings[k] = v
				}
			}
		}
	}
	return m
}

// Raw looks up a translation key, substituting {0}, {1}, ... arguments.
func (m *Messages) Raw(key string, args ...any) string {
	m.mu.RLock()
	tmpl, ok := m.strings[key]
	m.mu.RUnlock()
	if !ok {
		tmpl = key
	}
	return formatPlaceholders(tmpl, args...)
}

// Prefixed formats a key with FAWE's configured "prefix" caption into a plugin.Text component.
func (m *Messages) Prefixed(key string, args ...any) plugin.Text {
	body := m.Raw(key, args...)
	m.mu.RLock()
	prefix := m.strings["prefix"]
	m.mu.RUnlock()
	if prefix == "" {
		prefix = "&8(&4&lFAWE&8)&7 {0}"
	}
	return ParseLegacyText(formatPlaceholders(prefix, body))
}

// Info sends a prefixed FAWE info message to sender.
func (m *Messages) Info(sender plugin.CommandSender, text string) {
	if sender == nil {
		return
	}
	m.mu.RLock()
	prefix := m.strings["prefix"]
	m.mu.RUnlock()
	if prefix == "" {
		prefix = "&8(&4&lFAWE&8)&7 {0}"
	}
	sender.SendMessage(ParseLegacyText(formatPlaceholders(prefix, text)))
}

// InfoKey sends a translated prefixed message to sender.
func (m *Messages) InfoKey(sender plugin.CommandSender, key string, args ...any) {
	if sender == nil {
		return
	}
	sender.SendMessage(m.Prefixed(key, args...))
}

// Error sends a red error message to sender.
func (m *Messages) Error(sender plugin.CommandSender, text string) {
	if sender == nil {
		return
	}
	m.mu.RLock()
	prefix := m.strings["prefix"]
	m.mu.RUnlock()
	if prefix == "" {
		prefix = "&8(&4&lFAWE&8)&7 {0}"
	}
	sender.SendMessage(ParseLegacyText(formatPlaceholders(prefix, "&c"+text)))
}

// ErrorKey sends a translated red error message to sender.
func (m *Messages) ErrorKey(sender plugin.CommandSender, key string, args ...any) {
	m.Error(sender, m.Raw(key, args...))
}

func formatPlaceholders(tmpl string, args ...any) string {
	out := tmpl
	for i, a := range args {
		ph := fmt.Sprintf("{%d}", i)
		out = strings.ReplaceAll(out, ph, fmt.Sprint(a))
	}
	return out
}

var legacyColorNames = map[byte]string{
	'0': "black",
	'1': "dark_blue",
	'2': "dark_green",
	'3': "dark_aqua",
	'4': "dark_red",
	'5': "dark_purple",
	'6': "gold",
	'7': "gray",
	'8': "dark_gray",
	'9': "blue",
	'a': "green",
	'b': "aqua",
	'c': "red",
	'd': "light_purple",
	'e': "yellow",
	'f': "white",
}

// ParseLegacyText converts '&' or '§' legacy formatting codes into a structured plugin.Text.
func ParseLegacyText(s string) plugin.Text {
	var children []plugin.Text
	var cur strings.Builder
	var style plugin.Style

	flush := func() {
		if cur.Len() == 0 {
			return
		}
		children = append(children, plugin.Text{Literal: cur.String(), Style: style})
		cur.Reset()
	}

	for i := 0; i < len(s); i++ {
		if s[i] == '&' || s[i] == 0xc2 && i+1 < len(s) && s[i+1] == 0xa7 {
			nextIdx := i + 1
			if s[i] == 0xc2 {
				nextIdx = i + 2
			}
			if nextIdx < len(s) {
				code := s[nextIdx]
				if code >= 'A' && code <= 'Z' {
					code += 'a' - 'A'
				}
				if col, ok := legacyColorNames[code]; ok {
					flush()
					style = plugin.Style{Color: col}
					i = nextIdx
					continue
				}
				switch code {
				case 'l':
					flush()
					t := true
					style.Bold = &t
					i = nextIdx
					continue
				case 'o':
					flush()
					t := true
					style.Italic = &t
					i = nextIdx
					continue
				case 'n':
					flush()
					t := true
					style.Underlined = &t
					i = nextIdx
					continue
				case 'm':
					flush()
					t := true
					style.Strikethrough = &t
					i = nextIdx
					continue
				case 'k':
					flush()
					t := true
					style.Obfuscated = &t
					i = nextIdx
					continue
				case 'r':
					flush()
					style = plugin.Style{}
					i = nextIdx
					continue
				}
			}
		}
		cur.WriteByte(s[i])
	}
	flush()
	if len(children) == 0 {
		return plugin.Plain("")
	}
	if len(children) == 1 {
		return children[0]
	}
	return plugin.Text{Children: children}
}
