package core

import (
	"fmt"
	"strings"
	"time"
)

// ASCIIFold turns a name into plain ASCII for the terminal: accents are
// dropped (á -> a, ñ -> n, ü -> u, ß -> ss), anything else becomes '?'.
func ASCIIFold(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 128:
			b.WriteRune(r)
		case strings.ContainsRune("ÀÁÂÃÄÅàáâãäå", r):
			b.WriteByte("Aa"[BoolIdx(r >= 'à')])
		case strings.ContainsRune("ÈÉÊËèéêë", r):
			b.WriteByte("Ee"[BoolIdx(r >= 'è')])
		case strings.ContainsRune("ÌÍÎÏìíîï", r):
			b.WriteByte("Ii"[BoolIdx(r >= 'ì')])
		case strings.ContainsRune("ÒÓÔÕÖØòóôõöø", r):
			b.WriteByte("Oo"[BoolIdx(r >= 'ò')])
		case strings.ContainsRune("ÙÚÛÜùúûü", r):
			b.WriteByte("Uu"[BoolIdx(r >= 'ù')])
		case r == 'Ñ':
			b.WriteByte('N')
		case r == 'ñ':
			b.WriteByte('n')
		case r == 'Ç':
			b.WriteByte('C')
		case r == 'ç':
			b.WriteByte('c')
		case r == 'ß':
			b.WriteString("ss")
		case r == '\u2019' || r == '\u2018':
			b.WriteByte('\'')
		case r == '\u2013' || r == '\u2014':
			b.WriteByte('-')
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

// HMS formats a duration as m:ss, or h:mm:ss.
func HMS(d time.Duration) string {
	s := int(d.Round(time.Second).Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// Wrap splits s into lines of at most n characters at spaces, keeping any
// leading indentation on continuation lines.
func Wrap(s string, n int) []string {
	if len(s) <= n {
		return []string{s}
	}
	indent := len(s) - len(strings.TrimLeft(s, " "))
	var out []string
	line := ""
	for _, w := range strings.Fields(s) {
		switch {
		case line == "":
			line = strings.Repeat(" ", indent) + w
		case len(line)+1+len(w) <= n:
			line += " " + w
		default:
			out = append(out, line)
			line = strings.Repeat(" ", indent) + w
		}
		for len(line) > n { // a single word longer than the line
			out = append(out, line[:n])
			line = line[n:]
		}
	}
	return append(out, line)
}
