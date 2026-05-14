package parser

import (
	"strings"
	"unicode"
)

type token struct {
	text   string
	upper  string
	quoted bool
}

func tokenize(input string) []token {
	var tokens []token
	runes := []rune(input)
	for i := 0; i < len(runes); {
		ch := runes[i]
		if unicode.IsSpace(ch) {
			i++
			continue
		}
		if ch == '-' && i+1 < len(runes) && runes[i+1] == '-' {
			i += 2
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
			continue
		}
		if ch == '/' && i+1 < len(runes) && runes[i+1] == '*' {
			i += 2
			for i+1 < len(runes) && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			if i+1 < len(runes) {
				i += 2
			}
			continue
		}
		if ch == '\'' {
			start := i
			i++
			for i < len(runes) {
				if runes[i] == '\'' {
					i++
					if i < len(runes) && runes[i] == '\'' {
						i++
						continue
					}
					break
				}
				i++
			}
			addToken(&tokens, string(runes[start:i]), true)
			continue
		}
		if ch == '"' {
			start := i
			i++
			var builder strings.Builder
			for i < len(runes) {
				if runes[i] == '"' {
					i++
					if i < len(runes) && runes[i] == '"' {
						builder.WriteRune('"')
						i++
						continue
					}
					break
				}
				builder.WriteRune(runes[i])
				i++
			}
			text := builder.String()
			if text == "" {
				text = string(runes[start:i])
			}
			tokens = append(tokens, token{text: text, upper: strings.ToUpper(text), quoted: true})
			continue
		}
		if isIdentStart(ch) {
			start := i
			i++
			for i < len(runes) && isIdentPart(runes[i]) {
				i++
			}
			addToken(&tokens, string(runes[start:i]), false)
			continue
		}
		if unicode.IsDigit(ch) {
			start := i
			i++
			for i < len(runes) && (unicode.IsDigit(runes[i]) || runes[i] == '.') {
				i++
			}
			addToken(&tokens, string(runes[start:i]), false)
			continue
		}
		addToken(&tokens, string(ch), false)
		i++
	}
	return tokens
}

func addToken(tokens *[]token, text string, quoted bool) {
	*tokens = append(*tokens, token{text: text, upper: strings.ToUpper(text), quoted: quoted})
}

func isIdentStart(ch rune) bool {
	return unicode.IsLetter(ch) || ch == '_'
}

func isIdentPart(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '$'
}

func tokenText(tokens []token) string {
	parts := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		parts = append(parts, tok.text)
	}
	return strings.Join(parts, " ")
}

func normalizeIdentifier(tok token) string {
	if tok.quoted {
		return tok.text
	}
	return strings.ToLower(tok.text)
}
