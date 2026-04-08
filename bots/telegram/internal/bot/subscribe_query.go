package bot

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// subscribeTeamQueryFromMessage returns the text after /subscribe or /subscribe@BotName
// to send as GET /teams?name=. Parsing uses a prefix strip (not strings.Fields on the whole
// message), so unusual spaces between the command and the team name still yield a query.
func subscribeTeamQueryFromMessage(messageText string) string {
	text := strings.TrimSpace(messageText)
	if len(text) < 2 || text[0] != '/' {
		return ""
	}
	rest := text[1:]
	sub := "subscribe"
	if len(rest) < len(sub) || !strings.EqualFold(rest[:len(sub)], sub) {
		return ""
	}
	i := len(sub)
	if i < len(rest) && rest[i] == '@' {
		i++
		for i < len(rest) {
			r, w := utf8.DecodeRuneInString(rest[i:])
			if r == utf8.RuneError || unicode.IsSpace(r) {
				break
			}
			i += w
		}
	}
	q := rest[i:]
	q = strings.TrimLeftFunc(q, unicode.IsSpace)
	return strings.TrimSpace(q)
}
