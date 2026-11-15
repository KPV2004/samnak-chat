package domain

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

const (
	maxEmojiRunes = 8
	maxEmojiBytes = 32
	// MaxReactionKinds is the most distinct emoji one message may collect.
	MaxReactionKinds = 20

	zeroWidthJoiner = '‍'
	keycap          = '⃣'
)

// NormalizeEmoji checks that s is one emoji (CHAT-18): symbols, marks such as
// variation selectors and skin tones, joiners and tag characters, plus a
// digit, # or * when it forms a keycap. Plain text is rejected so reactions
// cannot carry messages.
func NormalizeEmoji(s string) (string, error) {
	bad := func() (string, error) {
		return "", fmt.Errorf("emoji must be a single emoji character: %w", app.ErrInvalid)
	}
	if s == "" || len(s) > maxEmojiBytes || !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxEmojiRunes {
		return bad()
	}
	isKeycap := strings.ContainsRune(s, keycap)
	for _, r := range s {
		switch {
		case unicode.IsSymbol(r), unicode.IsMark(r), r == zeroWidthJoiner, r >= 0xE0020 && r <= 0xE007F:
		case isKeycap && (r == '#' || r == '*' || (r >= '0' && r <= '9')):
		default:
			return bad()
		}
	}
	return s, nil
}
