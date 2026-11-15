package database

import "strings"

// LikeEscape is the ESCAPE clause that LikeContains patterns are written for.
// Use it as `column LIKE ? ESCAPE '!'`.
const LikeEscape = "ESCAPE '!'"

var likeEscaper = strings.NewReplacer(`!`, `!!`, `%`, `!%`, `_`, `!_`)

// LikeContains returns a LIKE pattern that matches value anywhere in a
// column, with value's own '%', '_' and '!' matched literally rather than as
// wildcards. The query must say LikeEscape after the placeholder.
func LikeContains(value string) string {
	return "%" + likeEscaper.Replace(value) + "%"
}
