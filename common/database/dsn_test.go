package database

import (
	"strings"
	"testing"
)

// The startup line printed the DSN whole, so every deployment wrote its
// database password into its own logs - readable by anyone with the log file, a
// log shipper, or a screenshot of the terminal.
func TestRedactDSNKeepsThePasswordOut(t *testing.T) {
	const secret = "s3cr3t-do-not-log"

	cases := map[string]string{
		"mysql":         "motor:" + secret + "@tcp(db.example.com:3306)/motor?charset=utf8mb4&parseTime=True",
		"postgres":      "postgres://motor:" + secret + "@db.example.com:5432/motor?sslmode=disable",
		"sqlserver":     "sqlserver://motor:" + secret + "@db.example.com:1433?database=motor",
		"empty-ish pwd": "motor:@tcp(db.example.com:3306)/motor",
	}

	for name, dsn := range cases {
		t.Run(name, func(t *testing.T) {
			got := redactDSN(dsn)
			if strings.Contains(got, secret) {
				t.Fatalf("password survived redaction: %s", got)
			}
			// Still has to be useful: the host is what makes the line worth logging.
			if !strings.Contains(got, "db.example.com") {
				t.Errorf("host was lost, the line no longer says anything: %s", got)
			}
			if !strings.Contains(got, "motor") {
				t.Errorf("username was lost: %s", got)
			}
		})
	}
}

// sqlite has no credential to hide, and its path is the useful part.
func TestRedactDSNLeavesAPathAlone(t *testing.T) {
	const path = "./motor-db.db"
	if got := redactDSN(path); got != path {
		t.Errorf("redactDSN(%q) = %q, want it unchanged", path, got)
	}
}

// An unparseable string might still hold a password, so it is not echoed.
func TestRedactDSNSaysNothingAboutWhatItCannotParse(t *testing.T) {
	got := redactDSN("://not a url at all:hunter2@")
	if strings.Contains(got, "hunter2") {
		t.Fatalf("password survived: %s", got)
	}
}

func TestRedactDSNHandlesEmpty(t *testing.T) {
	if got := redactDSN(""); got != "" {
		t.Errorf("redactDSN(\"\") = %q", got)
	}
}
