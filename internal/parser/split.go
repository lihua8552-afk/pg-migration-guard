package parser

import "strings"

type rawStatement struct {
	sql       string
	startLine int
	endLine   int
}

func splitStatements(sql string) []rawStatement {
	var out []rawStatement
	runes := []rune(sql)
	start := 0
	startLine := 1
	line := 1
	inSingle := false
	inDouble := false
	inLineComment := false
	inBlockComment := false
	dollarTag := ""

	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}

		if ch == '\n' {
			line++
			if inLineComment {
				inLineComment = false
			}
			continue
		}
		if inLineComment {
			continue
		}
		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if dollarTag != "" {
			if strings.HasPrefix(string(runes[i:]), dollarTag) {
				i += len([]rune(dollarTag)) - 1
				dollarTag = ""
			}
			continue
		}
		if inSingle {
			if ch == '\'' {
				if next == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			if ch == '"' {
				if next == '"' {
					i++
					continue
				}
				inDouble = false
			}
			continue
		}
		if ch == '-' && next == '-' {
			inLineComment = true
			i++
			continue
		}
		if ch == '/' && next == '*' {
			inBlockComment = true
			i++
			continue
		}
		if ch == '\'' {
			inSingle = true
			continue
		}
		if ch == '"' {
			inDouble = true
			continue
		}
		if ch == '$' {
			if tag, ok := readDollarTag(runes[i:]); ok {
				dollarTag = tag
				i += len([]rune(tag)) - 1
				continue
			}
		}
		if ch == ';' {
			stmt, stmtLine := trimLeadingSQL(string(runes[start:i]), startLine)
			if stmt != "" {
				out = append(out, rawStatement{sql: stmt, startLine: stmtLine, endLine: line})
			}
			start = i + 1
			startLine = line
		}
	}
	if tail, tailLine := trimLeadingSQL(string(runes[start:]), startLine); tail != "" {
		out = append(out, rawStatement{sql: tail, startLine: tailLine, endLine: line})
	}
	return out
}

func trimLeadingSQL(sql string, baseLine int) (string, int) {
	line := baseLine
	runes := []rune(sql)
	i := 0
	for i < len(runes) {
		switch runes[i] {
		case ' ', '\t', '\r':
			i++
		case '\n':
			line++
			i++
		default:
			return strings.TrimSpace(string(runes[i:])), line
		}
	}
	return "", line
}

func readDollarTag(runes []rune) (string, bool) {
	if len(runes) == 0 || runes[0] != '$' {
		return "", false
	}
	for i := 1; i < len(runes); i++ {
		if runes[i] == '$' {
			return string(runes[:i+1]), true
		}
		if !(runes[i] == '_' || runes[i] >= 'A' && runes[i] <= 'Z' || runes[i] >= 'a' && runes[i] <= 'z' || runes[i] >= '0' && runes[i] <= '9') {
			return "", false
		}
	}
	return "", false
}
