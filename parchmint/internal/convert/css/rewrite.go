// Package css rewrites the resource references inside a stylesheet: the
// targets of url() functions and of @import rules. It is a lexical scanner,
// not a CSS parser — it understands comments, strings, and escapes well
// enough to find every reference and leave the rest of the text untouched.
package css

import (
	"strings"
)

// Rewrite returns css with every url() and @import target replaced by the
// result of fn. Targets are passed to fn unquoted and unescaped; the
// replacements are re-quoted on the way out. Returning the input unchanged
// makes Rewrite a pure collector, which is how the packing side uses it.
func Rewrite(css string, fn func(url string) string) string {
	var out strings.Builder
	out.Grow(len(css))

	for i := 0; i < len(css); {
		switch {
		case strings.HasPrefix(css[i:], "/*"):
			end := strings.Index(css[i+2:], "*/")
			if end < 0 {
				out.WriteString(css[i:])
				return out.String()
			}
			out.WriteString(css[i : i+2+end+2])
			i += 2 + end + 2

		case css[i] == '"' || css[i] == '\'':
			value, raw, ok := scanString(css[i:])
			if !ok {
				out.WriteString(css[i:])
				return out.String()
			}
			// A string is only a resource reference when it is the target of
			// an @import rule; anywhere else it is ordinary content.
			if isImportTarget(css[:i]) {
				out.WriteString(quote(fn(value)))
			} else {
				out.WriteString(raw)
			}
			i += len(raw)

		case hasURLPrefix(css, i):
			value, raw, ok := scanURL(css[i:])
			if !ok {
				out.WriteString(css[i:])
				return out.String()
			}
			out.WriteString("url(" + quote(fn(value)) + ")")
			i += len(raw)

		default:
			out.WriteByte(css[i])
			i++
		}
	}

	return out.String()
}

// URLs returns every url() and @import target in css, in source order.
func URLs(css string) []string {
	var found []string
	Rewrite(css, func(url string) string {
		found = append(found, url)
		return url
	})
	return found
}

// hasURLPrefix reports whether a url( token starts at css[i]. The preceding
// character must not be part of an identifier, so that properties like
// "background-url(" or a custom "myurl(" are not mistaken for one.
func hasURLPrefix(css string, i int) bool {
	if len(css)-i < 4 || !strings.EqualFold(css[i:i+4], "url(") {
		return false
	}
	if i == 0 {
		return true
	}
	return !isIdentChar(css[i-1])
}

func isIdentChar(c byte) bool {
	return c == '-' || c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
		c >= '0' && c <= '9' || c >= 0x80
}

// scanString reads a quoted string starting at s[0]. It returns the unescaped
// contents and the raw text consumed, including both quotes.
func scanString(s string) (value, raw string, ok bool) {
	quote := s[0]
	var buf strings.Builder

	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 < len(s) {
				// A backslash-newline is a line continuation and contributes
				// nothing to the value.
				if s[i+1] != '\n' {
					buf.WriteByte(s[i+1])
				}
				i++
			}
		case quote:
			return buf.String(), s[:i+1], true
		case '\n':
			// Unterminated string: CSS strings may not span raw newlines.
			return "", "", false
		default:
			buf.WriteByte(s[i])
		}
	}

	return "", "", false
}

// scanURL reads a url(...) token starting at s[0]. It returns the target and
// the raw text consumed, including "url(" and the closing parenthesis.
func scanURL(s string) (value, raw string, ok bool) {
	i := len("url(")
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	if i >= len(s) {
		return "", "", false
	}

	// Quoted form: url("...") — the string carries the target.
	if s[i] == '"' || s[i] == '\'' {
		value, str, ok := scanString(s[i:])
		if !ok {
			return "", "", false
		}
		i += len(str)
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		if i >= len(s) || s[i] != ')' {
			return "", "", false
		}
		return value, s[:i+1], true
	}

	// Unquoted form: everything up to the closing parenthesis, with
	// backslash escapes honoured.
	var buf strings.Builder
	for ; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 < len(s) {
				buf.WriteByte(s[i+1])
				i++
			}
		case ')':
			return strings.TrimRight(buf.String(), " \t\r\n\f"), s[:i+1], true
		default:
			buf.WriteByte(s[i])
		}
	}

	return "", "", false
}

// isImportTarget reports whether a string literal appearing at the end of
// prefix is the target of an @import rule, i.e. whether prefix ends with
// "@import" followed only by whitespace.
func isImportTarget(prefix string) bool {
	end := len(prefix)
	for end > 0 && isSpace(prefix[end-1]) {
		end--
	}

	const keyword = "@import"
	if end < len(keyword) {
		return false
	}
	return strings.EqualFold(prefix[end-len(keyword):end], keyword)
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f'
}

// quote wraps url in double quotes, escaping the characters that would
// otherwise terminate the string or the surrounding url() token.
func quote(url string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\a `,
		"\r", `\d `,
	)
	return `"` + replacer.Replace(url) + `"`
}
