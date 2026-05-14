package model

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

func ParseSeverity(value string) (Severity, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "high":
		return SeverityHigh, nil
	case "low":
		return SeverityLow, nil
	case "medium":
		return SeverityMedium, nil
	case "critical":
		return SeverityCritical, nil
	default:
		return "", fmt.Errorf("unknown severity %q", value)
	}
}

func (s Severity) Rank() int {
	switch s {
	case SeverityLow:
		return 1
	case SeverityMedium:
		return 2
	case SeverityHigh:
		return 3
	case SeverityCritical:
		return 4
	default:
		return 0
	}
}

type Finding struct {
	RuleID         string   `json:"rule_id" yaml:"rule_id"`
	Severity       Severity `json:"severity" yaml:"severity"`
	File           string   `json:"file" yaml:"file"`
	Line           int      `json:"line" yaml:"line"`
	Statement      string   `json:"statement" yaml:"statement"`
	Reason         string   `json:"reason" yaml:"reason"`
	Recommendation string   `json:"recommendation" yaml:"recommendation"`
	MetadataUsed   bool     `json:"metadata_used" yaml:"metadata_used"`
	AIExplanation  string   `json:"ai_explanation,omitempty" yaml:"ai_explanation,omitempty"`
}

type Result struct {
	Tool        string    `json:"tool"`
	GeneratedAt time.Time `json:"generated_at"`
	Findings    []Finding `json:"findings"`
	Warnings    []string  `json:"warnings,omitempty"`
}

func (r Result) MaxSeverity() Severity {
	max := Severity("")
	for _, finding := range r.Findings {
		if finding.Severity.Rank() > max.Rank() {
			max = finding.Severity
		}
	}
	return max
}

type FileAnalysis struct {
	Path                   string
	HasExplicitTransaction bool
	HasRollbackHints       bool
	Statements             []Statement
	Warnings               []string
}

type StatementKind string

const (
	KindUnknown     StatementKind = "unknown"
	KindParseError  StatementKind = "parse_error"
	KindTransaction StatementKind = "transaction"
	KindCreateIndex StatementKind = "create_index"
	KindAlterTable  StatementKind = "alter_table"
	KindDropTable   StatementKind = "drop_table"
	KindUpdate      StatementKind = "update"
	KindDelete      StatementKind = "delete"
	KindTruncate    StatementKind = "truncate"
)

type Statement struct {
	Kind          StatementKind
	Raw           string
	StartLine     int
	EndLine       int
	SchemaName    string
	TableName     string
	TableRefs     []TableRef
	IndexName     string
	Columns       []string
	Concurrently  bool
	Unique        bool
	HasWhere      bool
	InTransaction bool
	ParseError    string
	AlterActions  []AlterAction
}

func (s Statement) QualifiedTable() string {
	if s.SchemaName == "" {
		return s.TableName
	}
	return s.SchemaName + "." + s.TableName
}

func (s Statement) QualifiedTables() []string {
	if len(s.TableRefs) == 0 {
		if s.QualifiedTable() == "" {
			return nil
		}
		return []string{s.QualifiedTable()}
	}
	tables := make([]string, 0, len(s.TableRefs))
	for _, ref := range s.TableRefs {
		if ref.QualifiedName() != "" {
			tables = append(tables, ref.QualifiedName())
		}
	}
	return tables
}

type TableRef struct {
	SchemaName string
	TableName  string
}

func (r TableRef) QualifiedName() string {
	if r.SchemaName == "" {
		return r.TableName
	}
	return r.SchemaName + "." + r.TableName
}

type AlterActionType string

const (
	AlterDropColumn    AlterActionType = "drop_column"
	AlterRenameColumn  AlterActionType = "rename_column"
	AlterRenameTable   AlterActionType = "rename_table"
	AlterColumnType    AlterActionType = "alter_column_type"
	AlterSetNotNull    AlterActionType = "set_not_null"
	AlterAddColumn     AlterActionType = "add_column"
	AlterAddConstraint AlterActionType = "add_constraint"
)

type AlterAction struct {
	Type            AlterActionType
	Column          string
	NewName         string
	DataType        string
	ConstraintType  string
	HasNotNull      bool
	HasDefault      bool
	DefaultConstant bool
	Raw             string
}

type DBMetadata struct {
	PostgresVersionNum int64                    `json:"postgres_version_num,omitempty"`
	Tables             map[string]TableMetadata `json:"tables"`
	Ambiguous          map[string]bool          `json:"-"`
}

func NewDBMetadata() *DBMetadata {
	return &DBMetadata{Tables: map[string]TableMetadata{}}
}

func (m *DBMetadata) LookupTable(name string) (TableMetadata, bool) {
	if m == nil {
		return TableMetadata{}, false
	}
	key := normalizeMetaName(name)
	if table, ok := m.Tables[key]; ok {
		return table, true
	}
	if !strings.Contains(key, ".") {
		if table, ok := m.Tables["public."+key]; ok {
			return table, true
		}
		var match TableMetadata
		matches := 0
		for tableKey, table := range m.Tables {
			if strings.HasSuffix(tableKey, "."+key) {
				match = table
				matches++
			}
		}
		if matches == 1 {
			return match, true
		}
		if matches > 1 {
			if m.Ambiguous == nil {
				m.Ambiguous = map[string]bool{}
			}
			m.Ambiguous[key] = true
		}
	}
	return TableMetadata{}, false
}

func (m *DBMetadata) Warnings() []string {
	if m == nil || len(m.Ambiguous) == 0 {
		return nil
	}
	names := make([]string, 0, len(m.Ambiguous))
	for n := range m.Ambiguous {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, fmt.Sprintf("table %q is ambiguous across multiple schemas; metadata-driven rules were skipped for it. Qualify the migration with schema.table to enable metadata.", n))
	}
	return out
}

func normalizeMetaName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

type TableMetadata struct {
	Schema       string                    `json:"schema"`
	Name         string                    `json:"name"`
	RowsEstimate int64                     `json:"rows_estimate"`
	SizeBytes    int64                     `json:"size_bytes"`
	Columns      map[string]ColumnMetadata `json:"columns"`
	Indexes      []IndexMetadata           `json:"indexes"`
	Constraints  []ConstraintMetadata      `json:"constraints"`
}

func (t TableMetadata) QualifiedName() string {
	if t.Schema == "" {
		return t.Name
	}
	return t.Schema + "." + t.Name
}

type ColumnMetadata struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
	NotNull  bool   `json:"not_null"`
}

type IndexMetadata struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Def     string   `json:"def"`
}

type ConstraintMetadata struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
