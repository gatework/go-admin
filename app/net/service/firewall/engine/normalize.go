// Package engine implements bounded, offline firewall workbench calculations.
// It consumes immutable firemon graphs and has no transport or database access.
package engine

import (
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/gatework/motor/common/errs"

	"github.com/gatework/firemon/flow"
	"github.com/gatework/motor/app/net/models/firewall/types"
)

// MaxRows limits input rows accepted by normalization.
const MaxRows = 1000

// MaxDesignRows limits rows considered in one policy design.
const MaxDesignRows = 100

// MaxRegions bounds intermediate traffic partitions.
const MaxRegions = 4096

// BudgetErr reports a calculation exceeding a configured bound.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var BudgetErr = errs.New(errs.KindInvalid, "firewall.invalid_input", "")

//revive:enable:error-naming

// InputErr reports malformed or incomplete firewall input.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var InputErr = errs.New(errs.KindInvalid, "firewall.invalid_input", "")

//revive:enable:error-naming
var digits = regexp.MustCompile(`^[0-9]+$`)
var numericAddress = regexp.MustCompile(`^[0-9][0-9./:-]*$`)
var nativeName = regexp.MustCompile(`^[\pL\pN_][\pL\pN_.:@/\\ -]*$`)

// Normalize keeps every original row and validates fragments independently.
// Empty cells are unresolved. Only an explicit any token is a wildcard.
func Normalize(input types.Input) (types.Normalized, error) {
	protocol := strings.ToLower(strings.TrimSpace(input.DefaultProtocol))
	if protocol == "" {
		protocol = "tcp"
	}
	if _, ok := protocolNumber(protocol); !ok || protocol == "any" {
		return types.Normalized{}, fmt.Errorf("%w: defaultProtocol", InputErr)
	}
	out := types.Normalized{DefaultProtocol: protocol, Rows: []types.NormalizedRow{}, Issues: []types.Notice{}}
	if len(input.Rows) > MaxRows || len(input.Text) > 8<<20 {
		return out, BudgetErr
	}
	rows := make([]inputLine, 0, len(input.Rows))
	for i, row := range input.Rows {
		rows = append(rows, inputLine{row: row, line: i + 1})
	}
	if strings.TrimSpace(input.Text) != "" {
		var err error
		rows, err = clipboardRows(input, rows)
		if err != nil {
			return out, err
		}
	}
	if len(rows) == 0 {
		out.Issues = append(out.Issues, types.Notice{Code: "empty_input"})
		return out, nil
	}
	keys := map[string]bool{}
	inputBytes := len(input.Text)
	for i, r := range rows {
		if i < len(input.Rows) {
			inputBytes += len(r.row.Source) + len(r.row.Destination) + len(r.row.Service) + len(r.row.Notes) + len(r.row.Key)
		}
		if inputBytes > 8<<20 {
			return out, BudgetErr
		}
		if len(fragments(r.row.Source, true))+len(fragments(r.row.Destination, true))+len(fragments(r.row.Service, false)) > MaxRegions {
			return out, BudgetErr
		}
		n := types.NormalizedRow{InputRow: r.row, Line: r.line, Corrections: []types.Notice{}, Issues: r.issues}
		if n.Issues == nil {
			n.Issues = []types.Notice{}
		}
		if n.Key == "" {
			n.Key = fmt.Sprintf("row-%d", i+1)
		}
		if keys[n.Key] {
			return out, fmt.Errorf("%w: duplicate row key", InputErr)
		}
		keys[n.Key] = true
		n.Sources = normalizeTerms(n.Source, "source", protocol, true, &n)
		n.Destinations = normalizeTerms(n.Destination, "destination", protocol, true, &n)
		n.Services = normalizeTerms(n.Service, "service", protocol, false, &n)
		addLiteralFamilyNotices(&n)
		n.Status = "ready"
		if len(n.Issues) > 0 {
			n.Status = "partial"
			if len(n.Sources) == 0 || len(n.Destinations) == 0 {
				n.Status = "invalid"
			}
		}
		out.Rows = append(out.Rows, n)
	}
	return out, nil
}

func addLiteralFamilyNotices(n *types.NormalizedRow) {
	if sources, known := literalAddressSet(n.Sources); known {
		if destinations, known := literalAddressSet(n.Destinations); known {
			n.Issues = joinNotices(n.Issues, familyNotices(*n, sources, destinations))
		}
	}
}

type inputLine struct {
	row    types.InputRow
	line   int
	issues []types.Notice
}

func clipboardRows(input types.Input, rows []inputLine) ([]inputLine, error) {
	delimiter := ','
	if strings.Contains(input.Text, "\t") {
		delimiter = '\t'
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(input.Text, "\ufeff")))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	columns := types.Columns{Source: 0, Destination: 1, Service: 2}
	if input.Columns != nil {
		columns = *input.Columns
	}
	firstRecord := true
	for {
		cells, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: clipboard format", InputErr)
		}
		line, _ := reader.FieldPos(0)
		if len(cells) == 1 && input.Columns == nil {
			cells = whitespaceColumns(cells[0])
		}
		if len(cells) == 1 && strings.TrimSpace(cells[0]) == "" {
			continue
		}
		if firstRecord {
			firstRecord = false
			if isHeader(cells, columns) {
				continue
			}
		}
		cell := func(i int) string {
			if i >= 0 && i < len(cells) {
				return cells[i]
			}
			return ""
		}
		row := types.InputRow{Source: cell(columns.Source), Destination: cell(columns.Destination), Service: cell(columns.Service)}
		if columns.Notes != nil {
			row.Notes = cell(*columns.Notes)
		} else if input.Columns == nil && len(cells) > 3 {
			row.Notes = cells[3]
		}
		issues := []types.Notice{}
		if len(cells) < 3 {
			issues = append(issues, types.Notice{Code: "missing_columns", Token: strings.Join(cells, string(delimiter))})
		}
		rows = append(rows, inputLine{row: row, line: line, issues: issues})
		if len(rows) > MaxRows {
			return nil, BudgetErr
		}
	}
	return rows, nil
}

func isHeader(cells []string, c types.Columns) bool {
	match := func(i int, choices ...string) bool {
		if i < 0 || i >= len(cells) {
			return false
		}
		v := strings.ToLower(strings.TrimSpace(cells[i]))
		return slices.Contains(choices, v)
	}
	return match(c.Source, "source", "src", "source address", "源", "源地址", "源ip") && match(c.Destination, "destination", "dst", "destination address", "目的", "目的地址", "目标地址", "目标ip") && match(c.Service, "service", "services", "port", "ports", "protocol/port", "服务", "端口", "协议/端口", "服务/端口", "协议端口", "业务服务")
}

func fragments(raw string, address bool) []string {
	raw = strings.NewReplacer("，", ",", "；", ";", "\r", " ", "\n", " ", "\t", " ").Replace(raw)
	var out []string
	// Explicit list delimiters are boundaries. Only whitespace can join an
	// address and dotted mask, or a protocol and destination port.
	for _, chunk := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' }) {
		fields := strings.FieldsFunc(chunk, unicode.IsSpace)
		for i := 0; i < len(fields); i++ {
			p := fields[i]
			if i+1 < len(fields) {
				if address {
					if a, e := netip.ParseAddr(p); e == nil && a.Is4() && strings.Count(fields[i+1], ".") == 3 {
						if _, e = netip.ParseAddr(fields[i+1]); e == nil && (strings.HasPrefix(fields[i+1], "255.") || fields[i+1] == "0.0.0.0" || maskPrefix(p+"/"+fields[i+1]) != "") {
							p += "/" + fields[i+1]
							i++
						}
					}
				} else if _, ok := protocolNumber(strings.ToLower(p)); ok && !digits.MatchString(p) && !strings.Contains(p, "/") && portLike(fields[i+1]) {
					p += "/" + fields[i+1]
					i++
				}
			}
			out = append(out, p)
		}
	}
	return out
}
func normalizeTerms(raw, field, protocol string, address bool, n *types.NormalizedRow) []types.Term {
	terms := []types.Term{}
	if strings.TrimSpace(raw) == "" {
		n.Issues = append(n.Issues, types.Notice{Field: field, Code: "required", Token: raw})
		return terms
	}
	seen := map[string]bool{}
	for _, token := range fragments(raw, address) {
		var term types.Term
		var ok bool
		if address {
			term, ok = parseAddress(token)
		} else {
			term, ok = parseService(token, protocol)
		}
		if !ok {
			n.Issues = append(n.Issues, types.Notice{Field: field, Token: token, Code: "invalid_token"})
			continue
		}
		if token != term.Value {
			code := "normalized"
			if !address && portLike(token) {
				code = "default_protocol"
			}
			n.Corrections = append(n.Corrections, types.Notice{Field: field, Token: token, Code: code, Value: term.Value})
		}
		k := term.Kind + ":" + term.Value
		if !seen[k] {
			terms = append(terms, term)
			seen[k] = true
		}
	}
	return terms
}
func parseAddress(token string) (types.Term, bool) {
	token = strings.TrimSpace(token)
	if strings.EqualFold(token, "any") || strings.EqualFold(token, "all") {
		return types.Term{Value: "any", Kind: "any"}, true
	}
	if r, ok := addressRange(token); ok {
		value := rangeString(r)
		if p, e := netip.ParsePrefix(token); e == nil {
			value = p.Masked().String()
		}
		if !strings.Contains(token, "-") && !strings.Contains(token, "/") {
			value = r.First.String() + "/" + strconv.Itoa(r.First.BitLen())
		}
		if strings.Contains(token, "/") && strings.Count(strings.SplitN(token, "/", 2)[1], ".") == 3 {
			value = maskPrefix(token)
		}
		return types.Term{Value: value, Kind: "address"}, true
	}
	if pieces := strings.SplitN(token, "/", 2); len(pieces) == 2 {
		if _, err := netip.ParseAddr(pieces[0]); err == nil {
			return types.Term{}, false
		}
	}
	if numericAddress.MatchString(token) || strings.ContainsAny(token, "[]") || strings.Contains(token, "/") && !strings.Contains(token, "::") {
		return types.Term{}, false
	}
	if nativeName.MatchString(token) {
		return types.Term{Value: token, Kind: "name"}, true
	}
	return types.Term{}, false
}
func maskPrefix(token string) string {
	p := strings.SplitN(token, "/", 2)
	a, err := netip.ParseAddr(p[0])
	if err != nil || !a.Is4() {
		return ""
	}
	m := net.ParseIP(p[1])
	if m == nil || m.To4() == nil {
		return ""
	}
	bits, size := net.IPMask(m.To4()).Size()
	if size != 32 {
		return ""
	}
	return netip.PrefixFrom(a, bits).Masked().String()
}
func addressRange(token string) (flow.AddressRange, bool) {
	if p, e := netip.ParsePrefix(token); e == nil {
		r, e := flow.PrefixRange(p)
		return r, e == nil
	}
	if strings.Contains(token, "/") {
		if p := maskPrefix(token); p != "" {
			r, e := flow.PrefixRange(netip.MustParsePrefix(p))
			return r, e == nil
		}
		return flow.AddressRange{}, false
	}
	if a, e := netip.ParseAddr(token); e == nil && a.Zone() == "" {
		return flow.AddressRange{First: a, Last: a}, true
	}
	if p := strings.SplitN(token, "-", 2); len(p) == 2 {
		a, e := netip.ParseAddr(p[0])
		if e != nil {
			return flow.AddressRange{}, false
		}
		tail := p[1]
		if a.Is4() && digits.MatchString(tail) {
			octets := a.As4()
			last, e := strconv.Atoi(tail)
			if e != nil || last > 255 {
				return flow.AddressRange{}, false
			}
			octets[3] = byte(last)
			tail = netip.AddrFrom4(octets).String()
		}
		b, e := netip.ParseAddr(tail)
		r := flow.AddressRange{First: a, Last: b}
		return r, e == nil && r.Valid()
	}
	return flow.AddressRange{}, false
}
func rangeString(r flow.AddressRange) string {
	if r.First == r.Last {
		return r.First.String()
	}
	return r.First.String() + "-" + r.Last.String()
}
func protocolNumber(p string) (uint16, bool) {
	switch p {
	case "any":
		return 0, true
	case "tcp":
		return 6, true
	case "udp":
		return 17, true
	case "sctp":
		return 132, true
	case "icmp":
		return 1, true
	case "icmpv6", "ipv6-icmp":
		return 58, true
	case "gre":
		return 47, true
	case "esp":
		return 50, true
	case "ah":
		return 51, true
	}
	n, e := strconv.ParseUint(p, 10, 8)
	return uint16(n), e == nil
}
func portLike(p string) bool {
	if digits.MatchString(p) {
		return true
	}
	p2 := strings.Split(p, "-")
	return len(p2) == 2 && digits.MatchString(p2[0]) && digits.MatchString(p2[1])
}
func interval(p string, limit uint16) (flow.Interval, bool) {
	if p == "any" || p == "*" {
		return flow.Interval{Low: 0, High: limit}, true
	}
	ps := strings.Split(p, "-")
	if len(ps) > 2 {
		return flow.Interval{}, false
	}
	lo, e := strconv.ParseUint(ps[0], 10, 16)
	if e != nil {
		return flow.Interval{}, false
	}
	hi := lo
	if len(ps) == 2 {
		hi, e = strconv.ParseUint(ps[1], 10, 16)
	}
	return flow.Interval{Low: uint16(lo), High: uint16(hi)}, e == nil && lo <= hi && hi <= uint64(limit)
}
func parseService(token, defaultProtocol string) (types.Term, bool) {
	original := token
	token = strings.ToLower(token)
	if token == "any" || token == "all" {
		return types.Term{Value: "any", Kind: "any"}, true
	}
	names := map[string]string{"http": "tcp/80", "https": "tcp/443", "ssh": "tcp/22", "telnet": "tcp/23", "smtp": "tcp/25", "domain": "udp/53", "dns": "udp/53", "ntp": "udp/123", "snmp": "udp/161", "snmptrap": "udp/162", "rdp": "tcp/3389", "mysql": "tcp/3306", "postgresql": "tcp/5432"}
	if value, ok := names[token]; ok {
		return types.Term{Value: value, Kind: "service"}, true
	}
	if portLike(token) {
		token = defaultProtocol + "/" + token
	}
	if strings.Contains(token, ":") && !strings.Contains(token, "/") {
		token = strings.Replace(token, ":", "/", 1)
	}
	p := strings.Split(token, "/")
	if len(p) == 2 {
		if _, ok := protocolNumber(p[1]); ok && !digits.MatchString(p[1]) && portLike(p[0]) {
			p[0], p[1] = p[1], p[0]
			token = strings.Join(p, "/")
		}
	}
	if _, ok := protocolNumber(p[0]); ok && p[0] != "any" {
		if len(p) == 1 {
			return types.Term{Value: token, Kind: "service"}, true
		}
		if len(p) > 3 {
			return types.Term{}, false
		}
		limit := uint16(65535)
		if p[0] == "icmp" || p[0] == "icmpv6" || p[0] == "ipv6-icmp" {
			limit = 255
		}
		for _, port := range p[1:] {
			if _, ok := interval(port, limit); !ok {
				return types.Term{}, false
			}
		}
		return types.Term{Value: token, Kind: "service"}, true
	}
	if strings.ContainsAny(token, "/:*") || portLike(token) || digits.MatchString(token) {
		return types.Term{}, false
	}
	if nativeName.MatchString(original) {
		return types.Term{Value: original, Kind: "name"}, true
	}
	return types.Term{}, false
}

func whitespaceColumns(line string) []string {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return []string{line}
	}
	take := func() string {
		v := fields[0]
		fields = fields[1:]
		if len(fields) > 0 && maskPrefix(v+"/"+fields[0]) != "" {
			v += " " + fields[0]
			fields = fields[1:]
		}
		return v
	}
	source := take()
	if len(fields) < 2 {
		return []string{line}
	}
	destination := take()
	if len(fields) == 0 {
		return []string{line}
	}
	return []string{source, destination, strings.Join(fields, " ")}
}
