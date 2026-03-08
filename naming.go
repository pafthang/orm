package orm

import (
	"regexp"
	"strings"
)

// NamingStrategy controls model and field naming.
type NamingStrategy interface {
	TableName(typeName string) string
	ColumnName(fieldName string) string
}

// DefaultNamingStrategy uses snake_case names and basic English pluralization.
type DefaultNamingStrategy struct{}

func (DefaultNamingStrategy) TableName(typeName string) string {
	s := toSnake(typeName)
	switch {
	case strings.HasSuffix(s, "s"), strings.HasSuffix(s, "x"), strings.HasSuffix(s, "z"), strings.HasSuffix(s, "ch"), strings.HasSuffix(s, "sh"):
		return s + "es"
	case strings.HasSuffix(s, "y") && len(s) > 1 && !isVowel(rune(s[len(s)-2])):
		return s[:len(s)-1] + "ies"
	default:
		return s + "s"
	}
}

func (DefaultNamingStrategy) ColumnName(fieldName string) string {
	return toSnake(fieldName)
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func toSnake(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = matchFirstCap.ReplaceAllString(s, "${1}_${2}")
	s = matchAllCap.ReplaceAllString(s, "${1}_${2}")
	return strings.ToLower(s)
}

func isVowel(r rune) bool {
	switch r {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	default:
		return false
	}
}
