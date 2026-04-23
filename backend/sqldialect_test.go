package main

import "testing"

func TestRebind_SQLiteIdentity(t *testing.T) {
	currentDialect = dialectSQLite
	in := "SELECT * FROM x WHERE a = ? AND b = ?"
	if got := rb(in); got != in {
		t.Fatalf("sqlite: rb should be identity, got %q", got)
	}
}

func TestRebind_Postgres(t *testing.T) {
	currentDialect = dialectPostgres
	defer func() { currentDialect = dialectSQLite }()
	cases := map[string]string{
		"SELECT 1":                   "SELECT 1",
		"SELECT * WHERE a=?":         "SELECT * WHERE a=$1",
		"INSERT INTO t VALUES (?,?)": "INSERT INTO t VALUES ($1,$2)",
		"UPDATE t SET a=?, b=? WHERE c=?": "UPDATE t SET a=$1, b=$2 WHERE c=$3",
	}
	for in, want := range cases {
		if got := rb(in); got != want {
			t.Fatalf("rb(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPickDialect(t *testing.T) {
	cases := []struct {
		in  string
		d   dialect
		out string
	}{
		{"", dialectSQLite, ""},
		{"data/nast.db", dialectSQLite, "data/nast.db"},
		{"sqlite:///tmp/x.db", dialectSQLite, "tmp/x.db"},
		{"postgres://u:p@h:5432/db", dialectPostgres, "postgres://u:p@h:5432/db"},
		{"postgresql://h/db", dialectPostgres, "postgresql://h/db"},
	}
	for _, tc := range cases {
		d, rest := pickDialect(tc.in)
		if d != tc.d || rest != tc.out {
			t.Fatalf("pickDialect(%q) = (%q, %q), want (%q, %q)", tc.in, d, rest, tc.d, tc.out)
		}
	}
}
