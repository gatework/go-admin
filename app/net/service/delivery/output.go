package delivery

import "unicode/utf8"

// appendCapped appends as much of part to output as keeps it within limit
// bytes, cutting only on a rune boundary so stored output stays valid UTF-8.
// full reports whether all of part fit.
func appendCapped(output, part string, limit int) (string, bool) {
	room := limit - len(output)
	if len(part) <= room {
		return output + part, true
	}
	if room <= 0 {
		return output, false
	}
	cut := room
	for cut > 0 && !utf8.RuneStart(part[cut]) {
		cut--
	}
	return output + part[:cut], false
}
