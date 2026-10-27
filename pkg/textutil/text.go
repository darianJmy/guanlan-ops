package textutil

// Truncate limits Unicode code points and appends suffix only when truncated.
// The suffix is not included in limit. Negative limits are treated as zero.
func Truncate(value string, limit int, suffix string) string {
	runes := []rune(value)
	limit = max(0, limit)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + suffix
}
func Ellipsis(value string, limit int) string { return Truncate(value, limit, "...") }
func Clip(value string, limit int) string     { return Truncate(value, limit, "") }
