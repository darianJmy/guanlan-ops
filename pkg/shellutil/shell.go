package shellutil

import "strings"

func Quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
func NormalizeLineEndings(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.ReplaceAll(value, "\r", "\n")
}
func IsIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		valid := r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9'
		if !valid {
			return false
		}
		if i == 0 && r >= '0' && r <= '9' {
			return false
		}
	}
	return true
}
func Command(name string, args []string) string {
	parts := []string{Quote(name)}
	for _, arg := range args {
		parts = append(parts, Quote(arg))
	}
	return strings.Join(parts, " ")
}
