package postgres

import (
	"fmt"
	"strings"

	"github.com/lenchik/logmonitor/internal/repository"
)

type sqlFilter struct {
	where []string
	args  []any
}

func (f *sqlFilter) add(condition string, value any) {
	f.args = append(f.args, value)
	f.where = append(f.where, fmt.Sprintf(condition, len(f.args)))
}

func (f *sqlFilter) addSearch(columns []string, query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
	f.args = append(f.args, "%"+strings.ToLower(query)+"%")
	placeholder := fmt.Sprintf("$%d", len(f.args))
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		parts = append(parts, fmt.Sprintf("LOWER(%s) LIKE %s", column, placeholder))
	}
	f.where = append(f.where, "("+strings.Join(parts, " OR ")+")")
}

func (f sqlFilter) whereSQL() string {
	if len(f.where) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(f.where, " AND ")
}

func normalizePage(filter repository.ListOptions) repository.ListOptions {
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	return filter
}

func orderDirection(order string, defaultDirection string) string {
	if strings.EqualFold(strings.TrimSpace(order), "asc") {
		return "ASC"
	}
	if strings.EqualFold(strings.TrimSpace(order), "desc") {
		return "DESC"
	}
	return defaultDirection
}

func orderBy(sort string, allowed map[string]string, defaultSort string, order string, defaultDirection string) string {
	column, ok := allowed[sort]
	if !ok {
		column = allowed[defaultSort]
	}
	return " ORDER BY " + column + " " + orderDirection(order, defaultDirection)
}
