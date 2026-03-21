package db

import (
	"fmt"
	"strings"
)

// QueryBuilder provides a simple type-safe query builder to avoid raw SQL concatenation.
// Based on: docs/service-decomposition.md §3.3
//
// Usage:
//
//	q := NewSelect("subscribers", "supi", "gpsi", "auth_method").
//	    Where("supi = $1").
//	    OrderBy("created_at DESC").
//	    Limit(10)
//	sql := q.Build()
type QueryBuilder struct {
	operation  string
	table      string
	columns    []string
	setClauses []string
	conditions []string
	orderBy    string
	limit      int
	offset     int
	onConflict string
}

// NewSelect creates a SELECT query builder.
func NewSelect(table string, columns ...string) *QueryBuilder {
	if len(columns) == 0 {
		columns = []string{"*"}
	}
	return &QueryBuilder{
		operation: "SELECT",
		table:     table,
		columns:   columns,
	}
}

// NewInsert creates an INSERT query builder.
func NewInsert(table string, columns ...string) *QueryBuilder {
	return &QueryBuilder{
		operation: "INSERT",
		table:     table,
		columns:   columns,
	}
}

// NewUpdate creates an UPDATE query builder.
func NewUpdate(table string) *QueryBuilder {
	return &QueryBuilder{
		operation: "UPDATE",
		table:     table,
	}
}

// NewDelete creates a DELETE query builder.
func NewDelete(table string) *QueryBuilder {
	return &QueryBuilder{
		operation: "DELETE",
		table:     table,
	}
}

// Set adds a SET clause for UPDATE queries.
func (q *QueryBuilder) Set(clause string) *QueryBuilder {
	q.setClauses = append(q.setClauses, clause)
	return q
}

// Where adds a WHERE condition.
func (q *QueryBuilder) Where(condition string) *QueryBuilder {
	q.conditions = append(q.conditions, condition)
	return q
}

// OrderBy sets the ORDER BY clause.
func (q *QueryBuilder) OrderBy(clause string) *QueryBuilder {
	q.orderBy = clause
	return q
}

// Limit sets the LIMIT value.
func (q *QueryBuilder) Limit(n int) *QueryBuilder {
	q.limit = n
	return q
}

// Offset sets the OFFSET value.
func (q *QueryBuilder) Offset(n int) *QueryBuilder {
	q.offset = n
	return q
}

// OnConflict sets the ON CONFLICT clause for INSERT queries (upsert).
func (q *QueryBuilder) OnConflict(clause string) *QueryBuilder {
	q.onConflict = clause
	return q
}

// Build generates the SQL query string.
func (q *QueryBuilder) Build() string {
	var sb strings.Builder

	switch q.operation {
	case "SELECT":
		fmt.Fprintf(&sb, "SELECT %s FROM %s", strings.Join(q.columns, ", "), q.table)
	case "INSERT":
		placeholders := make([]string, len(q.columns))
		for i := range q.columns {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		}
		fmt.Fprintf(&sb, "INSERT INTO %s (%s) VALUES (%s)",
			q.table,
			strings.Join(q.columns, ", "),
			strings.Join(placeholders, ", "))
		if q.onConflict != "" {
			fmt.Fprintf(&sb, " ON CONFLICT %s", q.onConflict)
		}
	case "UPDATE":
		fmt.Fprintf(&sb, "UPDATE %s SET %s", q.table, strings.Join(q.setClauses, ", "))
	case "DELETE":
		fmt.Fprintf(&sb, "DELETE FROM %s", q.table)
	}

	if len(q.conditions) > 0 {
		fmt.Fprintf(&sb, " WHERE %s", strings.Join(q.conditions, " AND "))
	}

	if q.orderBy != "" {
		fmt.Fprintf(&sb, " ORDER BY %s", q.orderBy)
	}

	if q.limit > 0 {
		fmt.Fprintf(&sb, " LIMIT %d", q.limit)
	}

	if q.offset > 0 {
		fmt.Fprintf(&sb, " OFFSET %d", q.offset)
	}

	return sb.String()
}
