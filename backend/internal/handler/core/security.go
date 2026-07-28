package core

import (
	"fmt"
	"regexp"
	"strings"
)

var blockedSQLPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^\s*DROP\s+DATABASE`),
	regexp.MustCompile(`(?i)^\s*DROP\s+OWNED\s+BY`),
	regexp.MustCompile(`(?i)^\s*ALTER\s+ROLE`),
	regexp.MustCompile(`(?i)^\s*CREATE\s+ROLE`),
	regexp.MustCompile(`(?i)^\s*DROP\s+ROLE`),
	regexp.MustCompile(`(?i)^\s*GRANT\s+`),
	regexp.MustCompile(`(?i)^\s*REVOKE\s+`),
	regexp.MustCompile(`(?i)^\s*TRUNCATE\s+`),
	regexp.MustCompile(`(?i)^\s*COMMENT\s+ON\s+DATABASE`),
}

func IsBlockedSQL(sql string) bool {
	trimmed := strings.TrimSpace(sql)
	for _, pattern := range blockedSQLPatterns {
		if pattern.MatchString(trimmed) {
			return true
		}
	}
	return false
}

type WhereCondition struct {
	Column   string      `json:"column"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value,omitempty"`
}

func BuildWhereClauses(conditions []WhereCondition, startIdx int) ([]string, []interface{}, error) {
	clauses := make([]string, 0, len(conditions))
	args := make([]interface{}, 0)
	argIdx := startIdx

	for _, c := range conditions {
		if c.Column == "" {
			return nil, nil, fmt.Errorf("column name is required in WHERE condition")
		}
		if !ValidName.MatchString(c.Column) {
			return nil, nil, fmt.Errorf("invalid column name in WHERE: %s", c.Column)
		}

		switch strings.ToUpper(c.Operator) {
		case "=", "!=", ">", "<", ">=", "<=", "LIKE":
			clauses = append(clauses, fmt.Sprintf("%s %s $%d", QuoteIdent(c.Column), c.Operator, argIdx))
			args = append(args, c.Value)
			argIdx++
		case "IS NULL":
			clauses = append(clauses, fmt.Sprintf("%s IS NULL", QuoteIdent(c.Column)))
		case "IS NOT NULL":
			clauses = append(clauses, fmt.Sprintf("%s IS NOT NULL", QuoteIdent(c.Column)))
		default:
			return nil, nil, fmt.Errorf("unsupported operator: %s", c.Operator)
		}
	}

	if len(clauses) == 0 {
		return nil, nil, fmt.Errorf("at least one WHERE condition is required")
	}

	return clauses, args, nil
}
