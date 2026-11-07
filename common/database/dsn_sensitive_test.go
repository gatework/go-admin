package database

import (
	"strings"
	"testing"
)

func TestAuditKeywordDSNRedaction(t *testing.T) {
	input := "host=127.0.0.1 port=5432 user=app password=audit-only-password dbname=app sslmode=disable"
	if strings.Contains(redactDSN(input), "audit-only-password") {
		t.Fatal("PostgreSQL keyword DSN password survives startup-log redaction")
	}
}

func TestDSNQueryAndQuotedSecretsAreOmitted(t *testing.T) {
	for _, dsn := range []string{
		"host=localhost password='secret with spaces' dbname=test",
		"postgres://user@localhost/test?password=secret-value&sslmode=disable",
		"sqlserver://user@localhost?password=secret-value",
		"user:secret-value@tcp(localhost:3306)/test?password=secret-value",
		"user:secret://value@tcp(localhost:3306)/test",
		"postgres://secret-value@unix(/tmp/mysql.sock)/test",
	} {
		if strings.Contains(redactDSN(dsn), "secret") {
			t.Fatalf("secret retained: %s", redactDSN(dsn))
		}
	}
}
