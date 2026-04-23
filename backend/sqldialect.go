package main

import (
	"strconv"
	"strings"
)

// dialect distinguishes the two supported drivers. It is set once at
// boot inside openDB and then consulted by rb() to rewrite queries for
// Postgres (which uses $1, $2, … instead of SQLite's ? placeholders).
//
// Both drivers understand modern UPSERT syntax (ON CONFLICT … DO
// UPDATE), so we only diverge on placeholder style and on migration
// definitions.
type dialect string

const (
	dialectSQLite   dialect = "sqlite"
	dialectPostgres dialect = "postgres"
)

var currentDialect = dialectSQLite

// rb (rebind) rewrites a query string for the active dialect. For
// SQLite it returns the input verbatim. For Postgres it replaces each
// '?' with $N where N is the 1-based position. We intentionally do not
// parse string literals because no query in this codebase embeds a
// literal '?' inside a quoted string.
func rb(q string) string {
	if currentDialect != dialectPostgres {
		return q
	}
	if !strings.ContainsRune(q, '?') {
		return q
	}
	var b strings.Builder
	b.Grow(len(q) + 8)
	n := 0
	for i := 0; i < len(q); i++ {
		c := q[i]
		if c != '?' {
			b.WriteByte(c)
			continue
		}
		n++
		b.WriteByte('$')
		b.WriteString(strconv.Itoa(n))
	}
	return b.String()
}
