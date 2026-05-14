package report

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lihua8552-afk/pg-migration-guard/internal/model"
)

func Write(w io.Writer, format string, result model.Result) error {
	switch strings.ToLower(format) {
	case "", "text":
		return WriteText(w, result)
	case "markdown", "md":
		return WriteMarkdown(w, result)
	case "json":
		return WriteJSON(w, result)
	case "sarif":
		return WriteSARIF(w, result)
	default:
		return fmt.Errorf("unknown report format %q", format)
	}
}

func WriteText(w io.Writer, result model.Result) error {
	counts := countBySeverity(result.Findings)
	if _, err := fmt.Fprintf(w, "mguard found %d finding(s): critical=%d high=%d medium=%d low=%d\n",
		len(result.Findings), counts[model.SeverityCritical], counts[model.SeverityHigh], counts[model.SeverityMedium], counts[model.SeverityLow]); err != nil {
		return err
	}
	for _, warning := range result.Warnings {
		if _, err := fmt.Fprintf(w, "warning: %s\n", warning); err != nil {
			return err
		}
	}
	for _, finding := range sortedFindings(result.Findings) {
		if _, err := fmt.Fprintf(w, "\n[%s] %s %s:%d\n%s\nRecommendation: %s\n",
			strings.ToUpper(string(finding.Severity)), finding.RuleID, finding.File, finding.Line, finding.Reason, finding.Recommendation); err != nil {
			return err
		}
		if finding.AIExplanation != "" {
			if _, err := fmt.Fprintf(w, "AI: %s\n", finding.AIExplanation); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "SQL: %s\n", finding.Statement); err != nil {
			return err
		}
	}
	return nil
}

func WriteMarkdown(w io.Writer, result model.Result) error {
	counts := countBySeverity(result.Findings)
	if _, err := fmt.Fprintf(w, "# mguard report\n\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Found **%d** finding(s): critical=%d, high=%d, medium=%d, low=%d.\n\n",
		len(result.Findings), counts[model.SeverityCritical], counts[model.SeverityHigh], counts[model.SeverityMedium], counts[model.SeverityLow]); err != nil {
		return err
	}
	if len(result.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, "## Warnings"); err != nil {
			return err
		}
		for _, warning := range result.Warnings {
			if _, err := fmt.Fprintf(w, "- %s\n", warning); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	if len(result.Findings) == 0 {
		_, err := fmt.Fprintln(w, "No migration risks found.")
		return err
	}
	if _, err := fmt.Fprintln(w, "| Severity | Rule | Location | Reason | Recommendation |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "|---|---|---|---|---|"); err != nil {
		return err
	}
	for _, finding := range sortedFindings(result.Findings) {
		reason := escapeMarkdown(finding.Reason)
		recommendation := escapeMarkdown(finding.Recommendation)
		if finding.AIExplanation != "" {
			recommendation += "<br><br><strong>AI:</strong> " + escapeMarkdown(finding.AIExplanation)
		}
		if _, err := fmt.Fprintf(w, "| %s | `%s` | `%s:%d` | %s | %s |\n",
			finding.Severity, finding.RuleID, filepath.ToSlash(finding.File), finding.Line, reason, recommendation); err != nil {
			return err
		}
	}
	return nil
}

func WriteJSON(w io.Writer, result model.Result) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func WriteSARIF(w io.Writer, result model.Result) error {
	payload := sarifLog{
		Version: "2.1.0",
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "mguard",
				InformationURI: "https://github.com/lihua8552-afk/pg-migration-guard",
				Rules:          sarifRules(result.Findings),
			}},
			Results: sarifResults(result.Findings),
		}},
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}

func countBySeverity(findings []model.Finding) map[model.Severity]int {
	counts := map[model.Severity]int{}
	for _, finding := range findings {
		counts[finding.Severity]++
	}
	return counts
}

func sortedFindings(findings []model.Finding) []model.Finding {
	out := append([]model.Finding(nil), findings...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity.Rank() != out[j].Severity.Rank() {
			return out[i].Severity.Rank() > out[j].Severity.Rank()
		}
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

func escapeMarkdown(value string) string {
	value = strings.ReplaceAll(value, "\n", "<br>")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

type sarifLog struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	ShortDesc sarifMessage `json:"shortDescription"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

func sarifRules(findings []model.Finding) []sarifRule {
	seen := map[string]model.Finding{}
	for _, finding := range findings {
		if _, ok := seen[finding.RuleID]; !ok {
			seen[finding.RuleID] = finding
		}
	}
	rules := make([]sarifRule, 0, len(seen))
	for _, finding := range seen {
		rules = append(rules, sarifRule{
			ID:        finding.RuleID,
			Name:      finding.RuleID,
			ShortDesc: sarifMessage{Text: finding.Reason},
		})
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	return rules
}

func sarifResults(findings []model.Finding) []sarifResult {
	results := make([]sarifResult, 0, len(findings))
	for _, finding := range findings {
		results = append(results, sarifResult{
			RuleID:  finding.RuleID,
			Level:   sarifLevel(finding.Severity),
			Message: sarifMessage{Text: finding.Reason + " Recommendation: " + finding.Recommendation},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{URI: filepath.ToSlash(finding.File)},
					Region:           sarifRegion{StartLine: finding.Line},
				},
			}},
		})
	}
	return results
}

func sarifLevel(severity model.Severity) string {
	switch severity {
	case model.SeverityCritical, model.SeverityHigh:
		return "error"
	case model.SeverityMedium:
		return "warning"
	default:
		return "note"
	}
}
