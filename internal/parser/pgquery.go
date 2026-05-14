package parser

import (
	"fmt"
	"strings"

	pganalyze "github.com/pganalyze/pg_query_go/v6"
	pgquery "github.com/wasilibs/go-pgquery"

	"github.com/lihua8552-afk/mguard/internal/model"
)

func parseWithPGQuery(path, sql string) (model.FileAnalysis, error) {
	tree, err := pgquery.Parse(sql)
	if err != nil {
		return model.FileAnalysis{}, err
	}
	analysis := model.FileAnalysis{
		Path:             path,
		HasRollbackHints: hasRollbackHint(sql),
	}
	txDepth := 0
	for _, raw := range tree.GetStmts() {
		stmtSQL, startLine, endLine := rawSQL(sql, raw)
		stmt := model.Statement{
			Kind:      model.KindUnknown,
			Raw:       strings.TrimSpace(stmtSQL),
			StartLine: startLine,
			EndLine:   endLine,
		}
		stmt = statementFromPGNode(stmt, raw.GetStmt())
		stmt.InTransaction = txDepth > 0
		if stmt.Kind == model.KindTransaction {
			analysis.HasExplicitTransaction = true
		}
		analysis.Statements = append(analysis.Statements, stmt)
		txDepth = applyTransactionDelta(txDepth, stmt)
	}
	return analysis, nil
}

func statementFromPGNode(stmt model.Statement, node *pganalyze.Node) model.Statement {
	if node == nil {
		return stmt
	}
	if index := node.GetIndexStmt(); index != nil {
		stmt.Kind = model.KindCreateIndex
		stmt.IndexName = index.GetIdxname()
		stmt.Unique = index.GetUnique()
		stmt.Concurrently = index.GetConcurrent()
		setRelation(&stmt, index.GetRelation())
		for _, param := range index.GetIndexParams() {
			if elem := param.GetIndexElem(); elem != nil {
				switch {
				case elem.GetName() != "":
					stmt.Columns = append(stmt.Columns, strings.ToLower(elem.GetName()))
				case elem.GetIndexcolname() != "":
					stmt.Columns = append(stmt.Columns, strings.ToLower(elem.GetIndexcolname()))
				case elem.GetExpr() != nil:
					stmt.Columns = append(stmt.Columns, strings.ToLower(strings.TrimSpace(elem.GetExpr().String())))
				}
			}
		}
		return stmt
	}
	if alter := node.GetAlterTableStmt(); alter != nil {
		stmt.Kind = model.KindAlterTable
		setRelation(&stmt, alter.GetRelation())
		for _, cmdNode := range alter.GetCmds() {
			cmd := cmdNode.GetAlterTableCmd()
			if cmd == nil {
				continue
			}
			if action, ok := alterActionFromPG(cmd); ok {
				stmt.AlterActions = append(stmt.AlterActions, action)
			}
		}
		return stmt
	}
	if drop := node.GetDropStmt(); drop != nil {
		if drop.GetRemoveType() == pganalyze.ObjectType_OBJECT_TABLE {
			stmt.Kind = model.KindDropTable
			setTableRefs(&stmt, tableRefsFromDropObjects(drop.GetObjects()))
		}
		return stmt
	}
	if update := node.GetUpdateStmt(); update != nil {
		stmt.Kind = model.KindUpdate
		setRelation(&stmt, update.GetRelation())
		stmt.HasWhere = update.GetWhereClause() != nil
		return stmt
	}
	if deleteStmt := node.GetDeleteStmt(); deleteStmt != nil {
		stmt.Kind = model.KindDelete
		setRelation(&stmt, deleteStmt.GetRelation())
		stmt.HasWhere = deleteStmt.GetWhereClause() != nil
		return stmt
	}
	if truncate := node.GetTruncateStmt(); truncate != nil {
		stmt.Kind = model.KindTruncate
		refs := make([]model.TableRef, 0, len(truncate.GetRelations()))
		for _, relation := range truncate.GetRelations() {
			if relation.GetRangeVar() != nil {
				refs = append(refs, tableRefFromRangeVar(relation.GetRangeVar()))
			}
		}
		setTableRefs(&stmt, refs)
		return stmt
	}
	if rename := node.GetRenameStmt(); rename != nil {
		stmt.Kind = model.KindAlterTable
		setRelation(&stmt, rename.GetRelation())
		switch rename.GetRenameType() {
		case pganalyze.ObjectType_OBJECT_COLUMN:
			stmt.AlterActions = append(stmt.AlterActions, model.AlterAction{
				Type:    model.AlterRenameColumn,
				Column:  rename.GetSubname(),
				NewName: rename.GetNewname(),
				Raw:     stmt.Raw,
			})
		case pganalyze.ObjectType_OBJECT_TABLE:
			stmt.AlterActions = append(stmt.AlterActions, model.AlterAction{
				Type:    model.AlterRenameTable,
				NewName: rename.GetNewname(),
				Raw:     stmt.Raw,
			})
		}
		return stmt
	}
	if tx := node.GetTransactionStmt(); tx != nil {
		stmt.Kind = model.KindTransaction
		return stmt
	}
	return stmt
}

func alterActionFromPG(cmd *pganalyze.AlterTableCmd) (model.AlterAction, bool) {
	action := model.AlterAction{
		Column: cmd.GetName(),
		Raw:    cmd.String(),
	}
	switch cmd.GetSubtype() {
	case pganalyze.AlterTableType_AT_DropColumn:
		action.Type = model.AlterDropColumn
		return action, true
	case pganalyze.AlterTableType_AT_AlterColumnType:
		action.Type = model.AlterColumnType
		if def := cmd.GetDef(); def != nil && def.GetColumnDef() != nil {
			action.DataType = typeNameString(def.GetColumnDef().GetTypeName())
		}
		return action, true
	case pganalyze.AlterTableType_AT_SetNotNull:
		action.Type = model.AlterSetNotNull
		return action, true
	case pganalyze.AlterTableType_AT_AddColumn:
		def := cmd.GetDef()
		col := (*pganalyze.ColumnDef)(nil)
		if def != nil {
			col = def.GetColumnDef()
		}
		action.Type = model.AlterAddColumn
		if col != nil {
			action.Column = col.GetColname()
			action.DataType = typeNameString(col.GetTypeName())
			action.HasNotNull = col.GetIsNotNull()
			if rawDefault := col.GetRawDefault(); rawDefault != nil {
				action.HasDefault = true
				action.DefaultConstant = defaultNodeIsConstant(rawDefault)
			}
			for _, constraintNode := range col.GetConstraints() {
				constraint := constraintNode.GetConstraint()
				if constraint == nil {
					continue
				}
				if constraint.GetContype() == pganalyze.ConstrType_CONSTR_NOTNULL {
					action.HasNotNull = true
				}
				if constraint.GetContype() == pganalyze.ConstrType_CONSTR_DEFAULT {
					action.HasDefault = true
					if rawExpr := constraint.GetRawExpr(); rawExpr != nil {
						action.DefaultConstant = defaultNodeIsConstant(rawExpr)
					}
				}
			}
		}
		return action, true
	case pganalyze.AlterTableType_AT_AddConstraint:
		action.Type = model.AlterAddConstraint
		if def := cmd.GetDef(); def != nil && def.GetConstraint() != nil {
			action.ConstraintType = constraintTypeFromPG(def.GetConstraint())
		}
		return action, true
	}
	return model.AlterAction{}, false
}

func setRelation(stmt *model.Statement, relation *pganalyze.RangeVar) {
	if relation == nil {
		return
	}
	ref := tableRefFromRangeVar(relation)
	stmt.SchemaName = ref.SchemaName
	stmt.TableName = ref.TableName
}

func tableRefFromRangeVar(relation *pganalyze.RangeVar) model.TableRef {
	return model.TableRef{
		SchemaName: strings.ToLower(relation.GetSchemaname()),
		TableName:  strings.ToLower(relation.GetRelname()),
	}
}

func tableRefsFromDropObjects(objects []*pganalyze.Node) []model.TableRef {
	refs := make([]model.TableRef, 0, len(objects))
	for _, object := range objects {
		if ref, ok := tableRefFromDropObject(object); ok {
			refs = append(refs, ref)
		}
	}
	return refs
}

func tableRefFromDropObject(object *pganalyze.Node) (model.TableRef, bool) {
	parts := stringList(object)
	if len(parts) >= 2 {
		return model.TableRef{
			SchemaName: strings.ToLower(parts[len(parts)-2]),
			TableName:  strings.ToLower(parts[len(parts)-1]),
		}, true
	}
	if len(parts) == 1 {
		return model.TableRef{TableName: strings.ToLower(parts[0])}, true
	}
	return model.TableRef{}, false
}

func stringList(node *pganalyze.Node) []string {
	if node == nil {
		return nil
	}
	if list := node.GetList(); list != nil {
		var out []string
		for _, item := range list.GetItems() {
			out = append(out, stringList(item)...)
		}
		return out
	}
	if str := node.GetString_(); str != nil {
		return []string{str.GetSval()}
	}
	return nil
}

func typeNameString(typeName *pganalyze.TypeName) string {
	if typeName == nil {
		return ""
	}
	var parts []string
	for _, node := range typeName.GetNames() {
		parts = append(parts, stringList(node)...)
	}
	if len(parts) == 0 {
		return typeName.String()
	}
	name := strings.ToLower(strings.Join(parts, "."))
	var typmods []string
	for _, node := range typeName.GetTypmods() {
		if constant := node.GetAConst(); constant != nil && constant.GetIval() != nil {
			typmods = append(typmods, fmt.Sprintf("%d", constant.GetIval().GetIval()))
		}
	}
	if len(typmods) > 0 {
		name += "(" + strings.Join(typmods, ",") + ")"
	}
	return name
}

func constraintTypeFromPG(constraint *pganalyze.Constraint) string {
	switch constraint.GetContype() {
	case pganalyze.ConstrType_CONSTR_PRIMARY:
		return "primary_key"
	case pganalyze.ConstrType_CONSTR_UNIQUE:
		return "unique"
	default:
		return strings.ToLower(strings.TrimPrefix(constraint.GetContype().String(), "CONSTR_"))
	}
}

func defaultNodeIsConstant(node *pganalyze.Node) bool {
	if node == nil {
		return false
	}
	if node.GetAConst() != nil {
		return true
	}
	if cast := node.GetTypeCast(); cast != nil {
		return defaultNodeIsConstant(cast.GetArg())
	}
	return false
}

func rawSQL(sql string, raw *pganalyze.RawStmt) (string, int, int) {
	if raw == nil {
		return "", 1, 1
	}
	location := int(raw.GetStmtLocation())
	if location < 0 || location > len(sql) {
		location = 0
	}
	length := int(raw.GetStmtLen())
	end := len(sql)
	if length > 0 && location+length <= len(sql) {
		end = location + length
	}
	text, lineDelta := trimLeadingSQL(sql[location:end], lineForOffset(sql, location))
	startLine := lineDelta
	endLine := startLine + strings.Count(text, "\n")
	return text, startLine, endLine
}

func lineForOffset(sql string, offset int) int {
	if offset <= 0 {
		return 1
	}
	if offset > len(sql) {
		offset = len(sql)
	}
	return strings.Count(sql[:offset], "\n") + 1
}

func parsePGQueryOrFallback(path, sql string) model.FileAnalysis {
	analysis, err := parseWithPGQuery(path, sql)
	if err == nil {
		return analysis
	}
	fallback := parseWithTokenizer(path, sql)
	fallback.Warnings = append(fallback.Warnings, fmt.Sprintf("%s: PostgreSQL parser failed, used fallback tokenizer: %v", path, err))
	fallback.Statements = append([]model.Statement{{
		Kind:       model.KindParseError,
		Raw:        firstSQLPreview(sql),
		StartLine:  1,
		EndLine:    1,
		ParseError: err.Error(),
	}}, fallback.Statements...)
	return fallback
}

func applyTransactionDelta(depth int, stmt model.Statement) int {
	if stmt.Kind != model.KindTransaction {
		return depth
	}
	upper := strings.ToUpper(strings.TrimSpace(stmt.Raw))
	switch {
	case strings.HasPrefix(upper, "BEGIN"), strings.HasPrefix(upper, "START TRANSACTION"):
		return depth + 1
	case strings.HasPrefix(upper, "COMMIT"), strings.HasPrefix(upper, "END"), strings.HasPrefix(upper, "ROLLBACK"):
		if depth > 0 {
			return depth - 1
		}
	}
	return depth
}

func firstSQLPreview(sql string) string {
	for _, stmt := range splitStatements(sql) {
		if trimmed := strings.TrimSpace(stmt.sql); trimmed != "" {
			if len(trimmed) > 240 {
				return trimmed[:240] + "..."
			}
			return trimmed
		}
	}
	trimmed := strings.TrimSpace(sql)
	if len(trimmed) > 240 {
		return trimmed[:240] + "..."
	}
	return trimmed
}
