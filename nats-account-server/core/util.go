package core

import (
	"time"
)

// ShortKey returns the first 12 characters of a public key (or the key if it is < 12 long)
func ShortKey(s string) string {
	if s != "" && len(s) > 12 {
		s = s[0:12]
	}

	return s
}

// UnixToDate parses a unix date in UTC to a time
func UnixToDate(d int64) string {
	if d == 0 {
		return ""
	}
	when := time.Unix(d, 0).UTC()
	return when.Format("2006-01-02")
}
