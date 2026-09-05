package pr

import (
	"regexp"
	"strings"
)

// issueKeyRe matches issue keys like "SP-44": an uppercase prefix of 2-16
// letters/digits/underscores starting with a letter, then a number. Bare
// extraction is uppercase-only so lowercase words with dashes ("issue-44")
// are not mistaken for keys.
var issueKeyRe = regexp.MustCompile(`\b([A-Z][A-Z0-9_]{1,15})-([0-9]{1,9})\b`)

// closeVerbRe matches a close-intent keyword immediately preceding an issue
// key ("Closes SP-44", "fixes: SP-44", "Resolves sp-44"). The keyword must
// be a standalone word so "prefix SP-44" does not count as a fix; because
// the intent keyword is explicit, the key itself is matched case-
// insensitively and normalized to upper case.
var closeVerbRe = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s*:?\s+([A-Za-z][A-Za-z0-9_]{1,15}-[0-9]{1,9})\b`)

// ExtractIssueKeys returns every distinct issue key appearing in the text,
// in first-appearance order, normalized to upper case.
func ExtractIssueKeys(text string) []string {
	seen := map[string]bool{}
	var keys []string
	for _, m := range issueKeyRe.FindAllStringSubmatch(text, -1) {
		key := strings.ToUpper(m[1] + "-" + m[2])
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	return keys
}

// ParseCloseIntents returns the distinct issue keys that the text asks to
// close ("Closes/fixes/resolves KEY"), in first-appearance order.
func ParseCloseIntents(text string) []string {
	seen := map[string]bool{}
	var keys []string
	for _, m := range closeVerbRe.FindAllStringSubmatch(text, -1) {
		key := strings.ToUpper(m[1])
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	return keys
}

// SplitIssueKey separates a normalized key into its prefix and number parts.
func SplitIssueKey(key string) (string, int64) {
	m := issueKeyRe.FindStringSubmatch(key)
	if m == nil {
		return "", 0
	}
	var n int64
	for _, r := range m[2] {
		n = n*10 + int64(r-'0')
	}
	return strings.ToUpper(m[1]), n
}
