package telegram

import (
	"regexp"
	"strings"
)

// mdV2SpecialChars are all characters that must be escaped in Telegram MarkdownV2.
var mdV2SpecialChars = map[rune]bool{
	'*':  true,
	'_':  true,
	'[':  true,
	']':  true,
	'(':  true,
	')':  true,
	'~':  true,
	'`':  true,
	'>':  true,
	'<':  true,
	'#':  true,
	'+':  true,
	'-':  true,
	'=':  true,
	'|':  true,
	'{':  true,
	'}':  true,
	'.':  true,
	'!':  true,
	'\\': true,
}

type entityPattern struct {
	re    *regexp.Regexp
	open  string
	close string
}

// Match more specific entities first so they win over shorter delimiters.
var allEntityPatterns = []entityPattern{
	{re: regexp.MustCompile("(?s)```(?:[\\w]*\\n)?[\\s\\S]*?```"), open: "```", close: "```"},
	{re: regexp.MustCompile("`(?:[^`\\\n]|\\\\.)*`"), open: "`", close: "`"},
	{re: regexp.MustCompile(`(?m)\*\*>(?:[^\n]*)`), open: "**>", close: ""},
	{re: regexp.MustCompile(`(?m)^>(?:[^\n]*)`), open: ">", close: ""},
	{re: regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`), open: "!", close: ""},
	{re: regexp.MustCompile(`\[[^\]]*\]\([^)]*\)`), open: "[", close: ""},
	{re: regexp.MustCompile(`\|\|(?:[^|\\\n]|\\.)*\|\|`), open: "||", close: "||"},
	{re: regexp.MustCompile(`__(?:[^_\\\n]|\\.)*__`), open: "__", close: "__"},
	{re: regexp.MustCompile(`\*(?:[^*\\\n]|\\.)*\*`), open: "*", close: "*"},
	{re: regexp.MustCompile(`_(?:[^_\\\n]|\\.)*_`), open: "_", close: "_"},
	{re: regexp.MustCompile(`~(?:[^~\\\n]|\\.)*~`), open: "~", close: "~"},
}

// Entities that should keep inner content untouched.
var verbatimEntities = map[string]bool{
	"```": true,
	"`":   true,
	"**>": true,
	">":   true,
	"!":   true,
	"[":   true,
}

// markdownToTelegramMarkdownV2 converts Markdown-like text into Telegram MarkdownV2-safe text.
func markdownToTelegramMarkdownV2(text string) string {
	text = reHeading.ReplaceAllStringFunc(text, func(match string) string {
		sub := reHeading.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		return "*" + escapeMarkdownV2(sub[1]) + "*"
	})

	// Convert Markdown bold (**x**) into Telegram bold (*x*).
	text = reBoldStar.ReplaceAllString(text, "*$1*")

	return processMarkdownV2Text(text)
}

func processMarkdownV2Text(text string) string {
	if text == "" {
		return ""
	}

	bestStart := -1
	bestEnd := -1
	var bestPattern *entityPattern

	for i := range allEntityPatterns {
		p := &allEntityPatterns[i]
		loc := p.re.FindStringIndex(text)
		if loc == nil {
			continue
		}
		if bestStart == -1 || loc[0] < bestStart ||
			(loc[0] == bestStart && (loc[1]-loc[0]) > (bestEnd-bestStart)) {
			bestStart = loc[0]
			bestEnd = loc[1]
			bestPattern = p
		}
	}

	if bestPattern == nil {
		return escapeMarkdownV2(text)
	}

	var b strings.Builder
	if bestStart > 0 {
		b.WriteString(escapeMarkdownV2(text[:bestStart]))
	}

	matched := text[bestStart:bestEnd]
	if verbatimEntities[bestPattern.open] {
		b.WriteString(matched)
	} else {
		openLen := len(bestPattern.open)
		closeLen := len(bestPattern.close)
		inner := matched[openLen : len(matched)-closeLen]
		b.WriteString(bestPattern.open)
		b.WriteString(processMarkdownV2Text(inner))
		b.WriteString(bestPattern.close)
	}

	b.WriteString(processMarkdownV2Text(text[bestEnd:]))
	return b.String()
}

func escapeMarkdownV2(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if ch == '\\' && i+1 < len(runes) {
			b.WriteRune(ch)
			b.WriteRune(runes[i+1])
			i++
			continue
		}
		if mdV2SpecialChars[ch] {
			b.WriteByte('\\')
		}
		b.WriteRune(ch)
	}
	return b.String()
}
