package rules

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/lihua8552-afk/mguard/internal/model"
)

const (
	largeRows  = int64(1_000_000)
	largeBytes = int64(1 << 30)
)

func Evaluate(files []model.FileAnalysis, metadata *model.DBMetadata) []model.Finding {
	var findings []model.Finding
	for _, file := range files {
		for _, stmt := range file.Statements {
			fileFindings := evaluateStatement(file, stmt, metadata)
			for i := range fileFindings {
				fileFindings[i].File = file.Path
			}
			findings = append(findings, fileFindings...)
		}
	}
	return findings
}

func evaluateStatement(file model.FileAnalysis, stmt model.Statement, metadata *model.DBMetadata) []model.Finding {
	switch stmt.Kind {
	case model.KindParseError:
		return []model.Finding{finding("MGD000", model.SeverityCritical, stmt, false,
			fmt.Sprintf("PostgreSQL parser failed: %s", stmt.ParseError),
			"Fix the SQL syntax before merging. mguard cannot safely analyze an invalid migration.")}
	case model.KindCreateIndex:
		return evaluateCreateIndex(file, stmt, metadata)
	case model.KindAlterTable:
		return evaluateAlterTable(file, stmt, metadata)
	case model.KindDropTable:
		return evaluateDropTable(file, stmt, metadata)
	case model.KindUpdate, model.KindDelete:
		return evaluateDML(file, stmt, metadata)
	case model.KindTruncate:
		return evaluateTruncate(file, stmt, metadata)
	default:
		return nil
	}
}

func evaluateCreateIndex(file model.FileAnalysis, stmt model.Statement, metadata *model.DBMetadata) []model.Finding {
	var findings []model.Finding
	table, ok := metadata.LookupTable(stmt.QualifiedTable())
	if stmt.Concurrently && stmt.InTransaction {
		findings = append(findings, finding("MGD001", model.SeverityCritical, stmt, false,
			"CREATE INDEX CONCURRENTLY cannot run inside an explicit transaction block.",
			"Move this statement out of BEGIN/COMMIT or use a migration runner mode that disables transactional DDL for this migration."))
	}
	if !stmt.Concurrently {
		severity := model.SeverityMedium
		reason := "Plain CREATE INDEX can block writes while the index is built."
		metadataUsed := ok
		if ok && isLarge(table) {
			severity = model.SeverityHigh
			reason = fmt.Sprintf("Plain CREATE INDEX on large table %s may block writes; estimated rows=%d, size=%s.", table.QualifiedName(), table.RowsEstimate, bytesHuman(table.SizeBytes))
		}
		findings = append(findings, finding("MGD002", severity, stmt, metadataUsed, reason,
			"Use CREATE INDEX CONCURRENTLY for PostgreSQL, and ensure the migration is not wrapped in a transaction."))
	}
	if ok && len(stmt.Columns) > 0 && hasEquivalentIndex(table, stmt.Columns) {
		findings = append(findings, finding("MGD003", model.SeverityLow, stmt, true,
			"An existing index appears to cover the same column list.",
			"Review existing indexes before adding another one; duplicate indexes increase write cost and storage."))
	}
	return findings
}

func evaluateAlterTable(file model.FileAnalysis, stmt model.Statement, metadata *model.DBMetadata) []model.Finding {
	var findings []model.Finding
	table, ok := metadata.LookupTable(stmt.QualifiedTable())
	for _, action := range stmt.AlterActions {
		large := ok && isLarge(table)
		metadataUsed := ok
		switch action.Type {
		case model.AlterDropColumn:
			findings = append(findings, finding("MGD010", model.SeverityHigh, stmt, metadataUsed,
				fmt.Sprintf("Dropping column %s is destructive and can break application versions that still read or write it.", action.Column),
				"Use an expand-contract rollout: stop application reads/writes first, deploy, wait for old instances to drain, then drop the column in a later migration."))
			findings = append(findings, rollbackFinding(file, stmt, "MGD011")...)
		case model.AlterRenameColumn:
			findings = append(findings, finding("MGD012", model.SeverityHigh, stmt, metadataUsed,
				fmt.Sprintf("Renaming column %s to %s is not backwards compatible during rolling deploys.", action.Column, action.NewName),
				"Add the new column, dual-write or backfill, migrate readers, then remove the old column in a later release."))
			findings = append(findings, rollbackFinding(file, stmt, "MGD011")...)
		case model.AlterRenameTable:
			findings = append(findings, finding("MGD013", model.SeverityHigh, stmt, metadataUsed,
				fmt.Sprintf("Renaming table %s is not backwards compatible for old application versions.", stmt.QualifiedTable()),
				"Create a compatibility view or use an expand-contract rollout before removing the old table name."))
			findings = append(findings, rollbackFinding(file, stmt, "MGD011")...)
		case model.AlterColumnType:
			severity := model.SeverityMedium
			reason := fmt.Sprintf("Changing column %s type to %s can rewrite data or take strong locks.", action.Column, action.DataType)
			if large {
				severity = model.SeverityHigh
				reason = fmt.Sprintf("Changing column %s type on large table %s can rewrite data or take strong locks; estimated rows=%d, size=%s.", action.Column, table.QualifiedName(), table.RowsEstimate, bytesHuman(table.SizeBytes))
			}
			if ok {
				if column, exists := table.Columns[strings.ToLower(action.Column)]; exists && isLengthShortening(column.DataType, action.DataType) {
					findings = append(findings, finding("MGD018", model.SeverityHigh, stmt, true,
						fmt.Sprintf("Changing column %s from %s to %s can truncate existing data.", action.Column, column.DataType, action.DataType),
						"Check max existing value length first, clean or reject oversized values, then apply the type change during a controlled rollout."))
				}
			}
			findings = append(findings, finding("MGD014", severity, stmt, metadataUsed, reason,
				"Prefer adding a new column, backfilling in batches, switching application reads, then dropping the old column later."))
			findings = append(findings, rollbackFinding(file, stmt, "MGD011")...)
		case model.AlterSetNotNull:
			severity := model.SeverityMedium
			reason := fmt.Sprintf("SET NOT NULL on column %s scans the table to validate existing rows.", action.Column)
			if large {
				severity = model.SeverityHigh
				reason = fmt.Sprintf("SET NOT NULL on column %s scans large table %s; estimated rows=%d, size=%s.", action.Column, table.QualifiedName(), table.RowsEstimate, bytesHuman(table.SizeBytes))
			}
			findings = append(findings, finding("MGD015", severity, stmt, metadataUsed, reason,
				"Backfill nulls first, add a NOT VALID check constraint, validate it, then set NOT NULL during a controlled window."))
		case model.AlterAddColumn:
			if action.HasNotNull {
				severity := model.SeverityMedium
				reason := fmt.Sprintf("Adding NOT NULL column %s can fail on existing rows or require a table validation.", action.Column)
				if supportsFastDefault(metadata) && action.HasDefault && action.DefaultConstant {
					reason = fmt.Sprintf("Adding NOT NULL column %s with a constant default is generally fast on PostgreSQL 11+, but still needs rollout review for application compatibility.", action.Column)
				} else if large {
					severity = model.SeverityHigh
					reason = fmt.Sprintf("Adding NOT NULL column %s to large table %s is risky; estimated rows=%d, size=%s.", action.Column, table.QualifiedName(), table.RowsEstimate, bytesHuman(table.SizeBytes))
				}
				if action.HasDefault {
					reason += " The default should be checked against your PostgreSQL version and deployment plan."
				}
				findings = append(findings, finding("MGD016", severity, stmt, metadataUsed, reason,
					"Add the column nullable first, deploy writes for new rows, backfill in batches, then add and validate the NOT NULL constraint."))
			}
		case model.AlterAddConstraint:
			if action.ConstraintType == "unique" || action.ConstraintType == "primary_key" {
				severity := model.SeverityMedium
				reason := fmt.Sprintf("Adding a %s constraint may scan the table and block writes.", strings.ReplaceAll(action.ConstraintType, "_", " "))
				if large {
					severity = model.SeverityHigh
					reason = fmt.Sprintf("Adding a %s constraint on large table %s may scan data and block writes; estimated rows=%d, size=%s.", strings.ReplaceAll(action.ConstraintType, "_", " "), table.QualifiedName(), table.RowsEstimate, bytesHuman(table.SizeBytes))
				}
				findings = append(findings, finding("MGD017", severity, stmt, metadataUsed, reason,
					"Create a unique index concurrently first, then attach or add the constraint with the shortest possible lock window."))
			}
		}
	}
	return findings
}

func evaluateDropTable(file model.FileAnalysis, stmt model.Statement, metadata *model.DBMetadata) []model.Finding {
	var findings []model.Finding
	for _, tableName := range stmt.QualifiedTables() {
		_, ok := metadata.LookupTable(tableName)
		findings = append(findings, finding("MGD020", model.SeverityCritical, stmt, ok,
			fmt.Sprintf("Dropping table %s is destructive and not backwards compatible.", tableName),
			"Use an expand-contract rollout and keep the table until all old application versions and recovery needs are cleared."))
	}
	findings = append(findings, rollbackFinding(file, stmt, "MGD011")...)
	return findings
}

func evaluateDML(file model.FileAnalysis, stmt model.Statement, metadata *model.DBMetadata) []model.Finding {
	if stmt.HasWhere {
		return nil
	}
	table, ok := metadata.LookupTable(stmt.QualifiedTable())
	severity := model.SeverityHigh
	reason := fmt.Sprintf("%s without WHERE can rewrite or remove every row in %s.", strings.ToUpper(string(stmt.Kind)), stmt.QualifiedTable())
	if ok && isLarge(table) {
		severity = model.SeverityCritical
		reason = fmt.Sprintf("%s without WHERE targets large table %s; estimated rows=%d, size=%s.", strings.ToUpper(string(stmt.Kind)), table.QualifiedName(), table.RowsEstimate, bytesHuman(table.SizeBytes))
	}
	findings := []model.Finding{finding("MGD030", severity, stmt, ok, reason,
		"Use a WHERE clause and batch the change with explicit limits, monitoring, and a rollback plan.")}
	findings = append(findings, rollbackFinding(file, stmt, "MGD011")...)
	return findings
}

func evaluateTruncate(file model.FileAnalysis, stmt model.Statement, metadata *model.DBMetadata) []model.Finding {
	var findings []model.Finding
	for _, tableName := range stmt.QualifiedTables() {
		_, ok := metadata.LookupTable(tableName)
		findings = append(findings, finding("MGD031", model.SeverityCritical, stmt, ok,
			fmt.Sprintf("TRUNCATE on %s removes all rows and takes strong locks.", tableName),
			"Avoid TRUNCATE in application migrations; if this is intentional, isolate it with an explicit maintenance plan and backup."))
	}
	findings = append(findings, rollbackFinding(file, stmt, "MGD011")...)
	return findings
}

func rollbackFinding(file model.FileAnalysis, stmt model.Statement, ruleID string) []model.Finding {
	if file.HasRollbackHints {
		return nil
	}
	return []model.Finding{finding(ruleID, model.SeverityLow, stmt, false,
		"This irreversible migration file has no visible rollback/down-migration hint.",
		"Document the rollback strategy or add a down migration before merging.")}
}

func finding(ruleID string, severity model.Severity, stmt model.Statement, metadataUsed bool, reason, recommendation string) model.Finding {
	return model.Finding{
		RuleID:         ruleID,
		Severity:       severity,
		File:           "",
		Line:           stmt.StartLine,
		Statement:      compactSQL(stmt.Raw),
		Reason:         reason,
		Recommendation: recommendation,
		MetadataUsed:   metadataUsed,
	}
}

func isLarge(table model.TableMetadata) bool {
	return table.RowsEstimate >= largeRows || table.SizeBytes >= largeBytes
}

func supportsFastDefault(metadata *model.DBMetadata) bool {
	return metadata != nil && metadata.PostgresVersionNum >= 110000
}

func hasEquivalentIndex(table model.TableMetadata, columns []string) bool {
	want := normalizeColumns(columns)
	for _, idx := range table.Indexes {
		got := normalizeColumns(idx.Columns)
		if len(got) != len(want) {
			continue
		}
		matches := true
		for i := range want {
			if got[i] != want[i] {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func normalizeColumns(columns []string) []string {
	out := make([]string, 0, len(columns))
	for _, col := range columns {
		out = append(out, strings.ToLower(strings.TrimSpace(col)))
	}
	return out
}

func compactSQL(sql string) string {
	return strings.Join(strings.Fields(sql), " ")
}

func bytesHuman(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

var characterLengthPattern = regexp.MustCompile(`(?i)\b(?:character varying|varchar|character|char|bpchar)\s*\(\s*(\d+)\s*\)`)

func isLengthShortening(oldType, newType string) bool {
	oldLength, okOld := characterLength(oldType)
	newLength, okNew := characterLength(newType)
	return okOld && okNew && newLength < oldLength
}

func characterLength(dataType string) (int, bool) {
	match := characterLengthPattern.FindStringSubmatch(dataType)
	if len(match) != 2 {
		return 0, false
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	return value, true
}
