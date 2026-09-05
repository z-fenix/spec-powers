package notification

import "strings"

// MentionsName reports whether content mentions the given principal as
// @<name> (case-insensitive). Shared by agent run triggers and human
// mention notifications so both read the same mention syntax.
func MentionsName(content, name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	idx := 0
	for {
		at := strings.Index(content[idx:], "@")
		if at < 0 {
			return false
		}
		at += idx
		rest := content[at+1:]
		if len(rest) >= len(name) && strings.EqualFold(rest[:len(name)], name) {
			// The character after the name must not extend it into a
			// different word (e.g. @KunCodingX must not match KunCoding).
			after := rest[len(name):]
			if after == "" || !isNameByte(after[0]) {
				return true
			}
		}
		idx = at + 1
	}
}

func isNameByte(b byte) bool {
	return b == '_' || b == '-' ||
		(b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		b >= 0x80 // treat multibyte (e.g. CJK names) as part of the token
}
