package parser

import (
	"os"
	"strings"

	"github.com/lihua8552-afk/mguard/internal/model"
)

func ParseFile(path string) (model.FileAnalysis, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return model.FileAnalysis{}, err
	}
	return Parse(path, string(data)), nil
}

func Parse(path, sql string) model.FileAnalysis {
	return parsePGQueryOrFallback(path, sql)
}

func parseWithTokenizer(path, sql string) model.FileAnalysis {
	analysis := model.FileAnalysis{
		Path:             path,
		HasRollbackHints: hasRollbackHint(sql),
	}
	txDepth := 0
	for _, raw := range splitStatements(sql) {
		stmt := parseStatement(raw.sql, raw.startLine, raw.endLine)
		stmt.InTransaction = txDepth > 0
		if stmt.Kind == model.KindTransaction {
			analysis.HasExplicitTransaction = true
		}
		analysis.Statements = append(analysis.Statements, stmt)
		txDepth = applyTransactionDelta(txDepth, stmt)
	}
	return analysis
}

func hasRollbackHint(sql string) bool {
	for _, line := range strings.Split(sql, "\n") {
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "--") {
			comment := strings.TrimSpace(strings.TrimPrefix(line, "--"))
			commentUpper := strings.ToUpper(comment)
			if commentUpper == "+DOWN" ||
				commentUpper == "MIGRATE:DOWN" ||
				commentUpper == "MIGRATE DOWN" ||
				commentUpper == "DOWN MIGRATION" ||
				strings.HasPrefix(commentUpper, "+DOWN ") ||
				strings.HasPrefix(commentUpper, "MIGRATE:DOWN ") ||
				strings.HasPrefix(commentUpper, "MIGRATE DOWN ") {
				return true
			}
		}
		if strings.HasPrefix(upper, "/*") && strings.Contains(upper, "MIGRATE:DOWN") {
			return true
		}
	}
	return false
}

func parseStatement(raw string, startLine, endLine int) model.Statement {
	stmt := model.Statement{
		Kind:      model.KindUnknown,
		Raw:       strings.TrimSpace(raw),
		StartLine: startLine,
		EndLine:   endLine,
	}
	tokens := tokenize(raw)
	if len(tokens) == 0 {
		return stmt
	}
	if isTransactionControl(tokens) {
		stmt.Kind = model.KindTransaction
		return stmt
	}
	switch tokens[0].upper {
	case "CREATE":
		return parseCreate(stmt, tokens)
	case "ALTER":
		return parseAlter(stmt, tokens)
	case "DROP":
		return parseDrop(stmt, tokens)
	case "UPDATE":
		return parseUpdate(stmt, tokens)
	case "DELETE":
		return parseDelete(stmt, tokens)
	case "TRUNCATE":
		return parseTruncate(stmt, tokens)
	default:
		return stmt
	}
}

func isTransactionControl(tokens []token) bool {
	if len(tokens) == 0 {
		return false
	}
	return tokens[0].upper == "BEGIN" ||
		tokens[0].upper == "COMMIT" ||
		tokens[0].upper == "END" ||
		tokens[0].upper == "ROLLBACK" ||
		(len(tokens) > 1 && tokens[0].upper == "START" && tokens[1].upper == "TRANSACTION")
}

func parseCreate(stmt model.Statement, tokens []token) model.Statement {
	idx := indexOf(tokens, "INDEX", 1)
	if idx == -1 {
		return stmt
	}
	stmt.Kind = model.KindCreateIndex
	stmt.Unique = indexOf(tokens[:idx], "UNIQUE", 0) >= 0
	i := idx + 1
	if i < len(tokens) && tokens[i].upper == "CONCURRENTLY" {
		stmt.Concurrently = true
		i++
	}
	if i+2 < len(tokens) && tokens[i].upper == "IF" && tokens[i+1].upper == "NOT" && tokens[i+2].upper == "EXISTS" {
		i += 3
	}
	if i < len(tokens) {
		stmt.IndexName = normalizeIdentifier(tokens[i])
		i++
	}
	on := indexOf(tokens, "ON", i)
	if on == -1 {
		return stmt
	}
	tableStart := on + 1
	if tableStart < len(tokens) && tokens[tableStart].upper == "ONLY" {
		tableStart++
	}
	schema, table, next := parseQualifiedName(tokens, tableStart)
	stmt.SchemaName = schema
	stmt.TableName = table
	paren := indexOfText(tokens, "(", next)
	if paren != -1 {
		cols, _ := parseColumnList(tokens, paren)
		stmt.Columns = cols
	}
	return stmt
}

func parseAlter(stmt model.Statement, tokens []token) model.Statement {
	if len(tokens) < 3 || tokens[1].upper != "TABLE" {
		return stmt
	}
	stmt.Kind = model.KindAlterTable
	i := 2
	if i+1 < len(tokens) && tokens[i].upper == "IF" && tokens[i+1].upper == "EXISTS" {
		i += 2
	}
	if i < len(tokens) && tokens[i].upper == "ONLY" {
		i++
	}
	schema, table, next := parseQualifiedName(tokens, i)
	stmt.SchemaName = schema
	stmt.TableName = table
	if next >= len(tokens) {
		return stmt
	}
	for _, segment := range splitTopLevelCommas(tokens[next:]) {
		if action, ok := parseAlterAction(segment); ok {
			stmt.AlterActions = append(stmt.AlterActions, action)
		}
	}
	return stmt
}

func parseAlterAction(tokens []token) (model.AlterAction, bool) {
	if len(tokens) == 0 {
		return model.AlterAction{}, false
	}
	action := model.AlterAction{Raw: tokenText(tokens)}
	switch tokens[0].upper {
	case "DROP":
		i := 1
		if i < len(tokens) && tokens[i].upper == "COLUMN" {
			i++
		}
		if i+1 < len(tokens) && tokens[i].upper == "IF" && tokens[i+1].upper == "EXISTS" {
			i += 2
		}
		if i < len(tokens) {
			action.Type = model.AlterDropColumn
			action.Column = normalizeIdentifier(tokens[i])
			return action, true
		}
	case "RENAME":
		if len(tokens) >= 5 && tokens[1].upper == "COLUMN" && tokens[3].upper == "TO" {
			action.Type = model.AlterRenameColumn
			action.Column = normalizeIdentifier(tokens[2])
			action.NewName = normalizeIdentifier(tokens[4])
			return action, true
		}
		if len(tokens) >= 3 && tokens[1].upper == "TO" {
			action.Type = model.AlterRenameTable
			action.NewName = normalizeIdentifier(tokens[2])
			return action, true
		}
	case "ALTER":
		i := 1
		if i < len(tokens) && tokens[i].upper == "COLUMN" {
			i++
		}
		if i >= len(tokens) {
			return model.AlterAction{}, false
		}
		action.Column = normalizeIdentifier(tokens[i])
		i++
		if i < len(tokens) && tokens[i].upper == "TYPE" {
			action.Type = model.AlterColumnType
			action.DataType = strings.TrimSpace(tokenText(tokens[i+1:]))
			return action, true
		}
		if i+2 < len(tokens) && tokens[i].upper == "SET" && tokens[i+1].upper == "NOT" && tokens[i+2].upper == "NULL" {
			action.Type = model.AlterSetNotNull
			return action, true
		}
	case "ADD":
		i := 1
		if i < len(tokens) && tokens[i].upper == "COLUMN" {
			i++
		}
		if i < len(tokens) && tokens[i].upper == "CONSTRAINT" {
			action.Type = model.AlterAddConstraint
			if i+2 < len(tokens) {
				action.ConstraintType = constraintType(tokens[i+2:])
			}
			return action, true
		}
		if i < len(tokens) && (tokens[i].upper == "UNIQUE" || tokens[i].upper == "PRIMARY") {
			action.Type = model.AlterAddConstraint
			action.ConstraintType = constraintType(tokens[i:])
			return action, true
		}
		if i < len(tokens) {
			action.Type = model.AlterAddColumn
			action.Column = normalizeIdentifier(tokens[i])
			action.DataType = dataTypeUntil(tokens[i+1:])
			action.HasNotNull = containsSequence(tokens, "NOT", "NULL")
			if defaultIndex := indexOf(tokens, "DEFAULT", 0); defaultIndex >= 0 {
				action.HasDefault = true
				action.DefaultConstant = defaultExpressionIsConstant(tokens[defaultIndex+1:])
			}
			return action, true
		}
	}
	return model.AlterAction{}, false
}

func parseDrop(stmt model.Statement, tokens []token) model.Statement {
	if len(tokens) < 2 || tokens[1].upper != "TABLE" {
		return stmt
	}
	stmt.Kind = model.KindDropTable
	i := 2
	if i+1 < len(tokens) && tokens[i].upper == "IF" && tokens[i+1].upper == "EXISTS" {
		i += 2
	}
	setTableRefs(&stmt, parseTableRefs(tokens, i, map[string]bool{
		"CASCADE":  true,
		"RESTRICT": true,
	}))
	return stmt
}

func parseUpdate(stmt model.Statement, tokens []token) model.Statement {
	stmt.Kind = model.KindUpdate
	i := 1
	if i < len(tokens) && tokens[i].upper == "ONLY" {
		i++
	}
	schema, table, _ := parseQualifiedName(tokens, i)
	stmt.SchemaName = schema
	stmt.TableName = table
	stmt.HasWhere = indexOf(tokens, "WHERE", 0) >= 0
	return stmt
}

func parseDelete(stmt model.Statement, tokens []token) model.Statement {
	if len(tokens) < 2 || tokens[1].upper != "FROM" {
		return stmt
	}
	stmt.Kind = model.KindDelete
	i := 2
	if i < len(tokens) && tokens[i].upper == "ONLY" {
		i++
	}
	schema, table, _ := parseQualifiedName(tokens, i)
	stmt.SchemaName = schema
	stmt.TableName = table
	stmt.HasWhere = indexOf(tokens, "WHERE", 0) >= 0
	return stmt
}

func parseTruncate(stmt model.Statement, tokens []token) model.Statement {
	stmt.Kind = model.KindTruncate
	i := 1
	if i < len(tokens) && tokens[i].upper == "TABLE" {
		i++
	}
	setTableRefs(&stmt, parseTableRefs(tokens, i, map[string]bool{
		"CONTINUE": true,
		"RESTART":  true,
		"IDENTITY": true,
		"CASCADE":  true,
		"RESTRICT": true,
	}))
	return stmt
}

func parseTableRefs(tokens []token, start int, stopWords map[string]bool) []model.TableRef {
	if start >= len(tokens) {
		return nil
	}
	end := len(tokens)
	depth := 0
	for i := start; i < len(tokens); i++ {
		switch tokens[i].text {
		case "(":
			depth++
		case ")":
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 && stopWords[tokens[i].upper] {
				end = i
				i = len(tokens)
			}
		}
	}
	segments := splitTopLevelCommas(tokens[start:end])
	refs := make([]model.TableRef, 0, len(segments))
	for _, segment := range segments {
		segment = trimTokens(segment)
		if len(segment) == 0 {
			continue
		}
		if segment[0].upper == "ONLY" {
			segment = trimTokens(segment[1:])
		}
		schema, table, _ := parseQualifiedName(segment, 0)
		if table != "" {
			refs = append(refs, model.TableRef{SchemaName: schema, TableName: table})
		}
	}
	return refs
}

func setTableRefs(stmt *model.Statement, refs []model.TableRef) {
	stmt.TableRefs = refs
	if len(refs) > 0 {
		stmt.SchemaName = refs[0].SchemaName
		stmt.TableName = refs[0].TableName
	}
}

func parseQualifiedName(tokens []token, start int) (schema, table string, next int) {
	if start >= len(tokens) {
		return "", "", start
	}
	first := normalizeIdentifier(tokens[start])
	if start+2 < len(tokens) && tokens[start+1].text == "." {
		return first, normalizeIdentifier(tokens[start+2]), start + 3
	}
	return "", first, start + 1
}

func parseColumnList(tokens []token, open int) ([]string, int) {
	var cols []string
	depth := 0
	start := open + 1
	for i := open; i < len(tokens); i++ {
		if tokens[i].text == "(" {
			depth++
			continue
		}
		if tokens[i].text == ")" {
			depth--
			if depth == 0 {
				cols = appendColumn(cols, tokens[start:i])
				return cols, i + 1
			}
			continue
		}
		if depth == 1 && tokens[i].text == "," {
			cols = appendColumn(cols, tokens[start:i])
			start = i + 1
		}
	}
	return cols, len(tokens)
}

func appendColumn(cols []string, tokens []token) []string {
	tokens = trimTokens(tokens)
	if len(tokens) == 0 {
		return cols
	}
	if len(tokens) == 1 {
		return append(cols, normalizeIdentifier(tokens[0]))
	}
	return append(cols, strings.ToLower(tokenText(tokens)))
}

func splitTopLevelCommas(tokens []token) [][]token {
	var out [][]token
	start := 0
	depth := 0
	for i, tok := range tokens {
		switch tok.text {
		case "(":
			depth++
		case ")":
			if depth > 0 {
				depth--
			}
		case ",":
			if depth == 0 {
				out = append(out, trimTokens(tokens[start:i]))
				start = i + 1
			}
		}
	}
	out = append(out, trimTokens(tokens[start:]))
	return out
}

func trimTokens(tokens []token) []token {
	start := 0
	end := len(tokens)
	for start < end && strings.TrimSpace(tokens[start].text) == "" {
		start++
	}
	for end > start && strings.TrimSpace(tokens[end-1].text) == "" {
		end--
	}
	return tokens[start:end]
}

func constraintType(tokens []token) string {
	if len(tokens) == 0 {
		return "constraint"
	}
	if tokens[0].upper == "PRIMARY" {
		return "primary_key"
	}
	if tokens[0].upper == "UNIQUE" {
		return "unique"
	}
	if len(tokens) > 1 && tokens[1].upper == "PRIMARY" {
		return "primary_key"
	}
	if len(tokens) > 1 && tokens[1].upper == "UNIQUE" {
		return "unique"
	}
	return strings.ToLower(tokens[0].text)
}

func dataTypeUntil(tokens []token) string {
	stop := len(tokens)
	for i, tok := range tokens {
		switch tok.upper {
		case "DEFAULT", "NOT", "NULL", "CONSTRAINT", "CHECK", "REFERENCES", "COLLATE":
			stop = i
			return strings.TrimSpace(tokenText(tokens[:stop]))
		}
	}
	return strings.TrimSpace(tokenText(tokens[:stop]))
}

func defaultExpressionIsConstant(tokens []token) bool {
	tokens = defaultExpressionTokens(tokens)
	if len(tokens) == 0 {
		return false
	}
	if len(tokens) == 1 {
		return isConstantToken(tokens[0])
	}
	if len(tokens) == 2 && (tokens[0].text == "-" || tokens[0].text == "+") {
		return isNumericToken(tokens[1])
	}
	return false
}

func defaultExpressionTokens(tokens []token) []token {
	stop := len(tokens)
	for i, tok := range tokens {
		switch tok.upper {
		case "NOT", "CONSTRAINT", "CHECK", "REFERENCES", "COLLATE":
			stop = i
			return trimTokens(tokens[:stop])
		}
	}
	return trimTokens(tokens[:stop])
}

func isConstantToken(tok token) bool {
	return tok.quoted ||
		isNumericToken(tok) ||
		tok.upper == "TRUE" ||
		tok.upper == "FALSE" ||
		tok.upper == "NULL"
}

func isNumericToken(tok token) bool {
	if tok.text == "" {
		return false
	}
	dotSeen := false
	digitSeen := false
	for _, ch := range tok.text {
		switch {
		case ch >= '0' && ch <= '9':
			digitSeen = true
		case ch == '.' && !dotSeen:
			dotSeen = true
		default:
			return false
		}
	}
	return digitSeen
}

func containsSequence(tokens []token, first, second string) bool {
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i].upper == first && tokens[i+1].upper == second {
			return true
		}
	}
	return false
}

func indexOf(tokens []token, upper string, start int) int {
	for i := start; i < len(tokens); i++ {
		if tokens[i].upper == upper {
			return i
		}
	}
	return -1
}

func indexOfText(tokens []token, text string, start int) int {
	for i := start; i < len(tokens); i++ {
		if tokens[i].text == text {
			return i
		}
	}
	return -1
}
