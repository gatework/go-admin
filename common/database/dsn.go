package database

import (
	"github.com/go-sql-driver/mysql"
	"net/url"
	"strings"
)

// redactDSN logs only parsed connection metadata; unknown formats are omitted.
func redactDSN(dsn string) string {
	if dsn == "" {
		return ""
	}

	// A MySQL password may itself contain a URL scheme. Recognize its network
	// suffix first rather than treating password bytes as URL user information.
	if at := strings.LastIndex(dsn, "@"); at >= 0 {
		network := dsn[at+1:]
		if strings.HasPrefix(network, "tcp(") || strings.HasPrefix(network, "unix(") {
			if c, err := mysql.ParseDSN(dsn); err == nil {
				return c.User + ":***@" + c.Net + "(" + c.Addr + ")/" + c.DBName
			}
			return "[dsn]"
		}
	}
	// URL form, used by postgres and sqlserver.
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") || strings.HasPrefix(dsn, "sqlserver://") {
		u, err := url.Parse(dsn)
		if err != nil {
			// Unparseable and possibly holding a password: say nothing about it.
			return "[dsn]"
		}
		if u.User != nil {
			if _, hasPassword := u.User.Password(); hasPassword {
				u.User = url.UserPassword(u.User.Username(), "***")
			}
		}
		u.RawQuery = ""
		u.Fragment = ""
		return u.String()
	}

	// Parse MySQL rather than guessing where an escaped password ends.
	if strings.Contains(dsn, "@") {
		if c, err := mysql.ParseDSN(dsn); err == nil {
			return c.User + ":***@" + c.Net + "(" + c.Addr + ")/" + c.DBName
		}
	}
	// A plain SQLite file path has no connection options or credentials.
	if !strings.ContainsAny(dsn, "=?:@\t\n ") {
		return dsn
	}
	return "[dsn]"
}
