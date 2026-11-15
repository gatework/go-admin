package service

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/gatework/motor/app/codegen/models"
)

// mapGeneratorColumn preserves legacy MySQL field choices while giving
// PostgreSQL's strict scalar types matching Go and browser representations.
func mapGeneratorColumn(column *models.SysColumns, source models.DBColumns, driver string) error {
	column.IsIncrement = "0"
	if strings.Contains(strings.ToLower(source.Extra), "auto_increment") {
		column.IsIncrement, column.Increment = "1", true
	}
	if driver != "postgres" {
		switch {
		case strings.Contains(source.ColumnType, "int"):
			column.GoType, column.HTMLType = "string", "input"
			if column.PK {
				column.GoType = "int"
			}
		case strings.Contains(source.ColumnType, "timestamp"), strings.Contains(source.ColumnType, "datetime"):
			column.GoType, column.HTMLType = "time.Time", "datetime"
		default:
			column.GoType, column.HTMLType = "string", "input"
		}
		return nil
	}
	if column.PK && strings.Contains(strings.ToUpper(source.Extra), "GENERATED") {
		return fmt.Errorf("PostgreSQL primary key %s is a generated expression; code generation supports assigned keys and serial/identity keys only", source.ColumnName)
	}
	column.HTMLType = "input"
	switch source.DataType {
	case "int2", "int4":
		column.GoType, column.HTMLType = "int", "number"
	case "int8":
		column.GoType, column.HTMLType = "int64", "input"
	case "float4":
		column.GoType, column.HTMLType = "float32", "number"
	case "float8":
		column.GoType, column.HTMLType = "float64", "number"
	case "numeric", "varchar", "bpchar", "text", "uuid":
		column.GoType = "string"
	case "bool":
		column.GoType, column.HTMLType = "bool", "checkbox"
	case "date", "timestamp", "timestamptz":
		column.GoType, column.HTMLType = "time.Time", "datetime"
	case "bytea":
		column.GoType = "[]byte"
	default:
		return fmt.Errorf("PostgreSQL column %s has unsupported generator type %s; use a supported scalar type or implement its Go and form mapping first", source.ColumnName, source.ColumnType)
	}
	if column.PK && column.GoType != "int" && column.GoType != "int64" && column.GoType != "string" {
		return fmt.Errorf("PostgreSQL primary key %s has unsupported generator type %s", source.ColumnName, source.ColumnType)
	}
	if !column.PK && source.IsNullable == "YES" && column.GoType != "[]byte" {
		column.GoType = "*" + column.GoType
	}
	// Audit fields are emitted through the framework's embedded structs,
	// not through these per-column declarations.
	switch source.ColumnName {
	case "create_by", "update_by", "deleted_at":
		column.GoType = "int"
	case "created_at", "updated_at":
		column.GoType = "time.Time"
	}
	if strings.Contains(strings.ToUpper(source.Extra), "GENERATED") || (!column.PK && column.Increment) {
		column.IsInsert, column.IsEdit = "0", "0"
		column.Insert, column.Edit = false, false
	}
	return nil
}

var generatorNumericLiteral = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

var postgresGeneratorIdentifier = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)

func validatePostgresGeneratorIdentifier(name string) error {
	if !postgresGeneratorIdentifier.MatchString(name) {
		return fmt.Errorf("PostgreSQL database identifier %q is unsupported by generated search queries; use lowercase letters, digits and nonempty underscore-separated words", name)
	}
	return nil
}

var generatorIdentifier = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*(?:_[A-Za-z0-9]+)*$`)

func generatorNames(name string) (goName, jsonName string, err error) {
	if !generatorIdentifier.MatchString(name) {
		return "", "", fmt.Errorf("database identifier %q cannot be represented by the generator; use letters, digits and nonempty underscore-separated words", name)
	}
	for i, part := range strings.Split(name, "_") {
		title := strings.ToUpper(part[:1]) + part[1:]
		goName += generatorGoWord(part)
		if i == 0 {
			jsonName += strings.ToLower(part[:1]) + part[1:]
		} else {
			jsonName += title
		}
	}
	return goName, jsonName, nil
}

// generatorGoWord normalizes Go initialisms independently of JSON field names.
func generatorGoWord(word string) string {
	upper := strings.ToUpper(word)
	switch upper {
	case "ACLS", "APIS", "CPUS", "DBS", "DTOS", "FKS", "GUIDS", "IDS", "IPS", "PKS", "URIS", "URLS", "UUIDS":
		return strings.TrimSuffix(upper, "S") + "s"
	case "ACL", "API", "ASCII", "CPU", "CSS", "DB", "DNS", "DTO", "EOF", "FK",
		"GUID", "HTML", "HTTP", "HTTPS", "ID", "IP", "JSON", "MIME", "PK", "QPS",
		"RAM", "RPC", "SLA", "SMTP", "SQL", "SSH", "TCP", "TLS", "TTL", "UDP",
		"UI", "UID", "URI", "URL", "UTF8", "UUID", "VM", "XML", "XSRF", "XSS":
		return upper
	default:
		return strings.ToUpper(word[:1]) + word[1:]
	}
}

// Generated models embed these framework fields. Refuse an incompatible source
// before writing generator metadata instead of producing CRUD that fails later.
func validatePostgresAuditColumns(columns []models.DBColumns) error {
	byName := make(map[string]models.DBColumns, len(columns))
	for _, column := range columns {
		byName[column.ColumnName] = column
	}
	for _, name := range []string{"create_by", "update_by", "created_at", "updated_at", "deleted_at"} {
		column, ok := byName[name]
		if !ok {
			return fmt.Errorf("PostgreSQL code generation requires audit column %s because generated models embed ModelTime and ControlBy", name)
		}
		valid := false
		switch name {
		case "create_by", "update_by":
			valid = column.DataType == "int4" || column.DataType == "int8"
		case "created_at", "updated_at":
			valid = column.DataType == "timestamp" || column.DataType == "timestamptz"
		case "deleted_at":
			valid = column.DataType == "int8" && column.IsNullable == "NO"
		}
		if !valid || strings.Contains(strings.ToUpper(column.Extra), "GENERATED") || strings.Contains(strings.ToLower(column.Extra), "auto_increment") {
			return fmt.Errorf("PostgreSQL audit column %s has incompatible type %s or generated/nullability settings; create_by/update_by require integer/bigint, created_at/updated_at require timestamp, deleted_at requires non-null bigint", name, column.ColumnType)
		}
	}
	return nil
}

func validateTypedColumnDefault(column models.SysColumns) error {
	if column.DefaultValue == "" {
		return nil
	}
	kind := strings.TrimPrefix(column.GoType, "*")
	if (kind == "int" || kind == "int64" || kind == "float32" || kind == "float64") && !generatorNumericLiteral.MatchString(column.DefaultValue) {
		return fmt.Errorf("column %s defaultValue must be a decimal numeric literal", column.ColumnName)
	}
	var err error
	switch kind {
	case "int", "int64":
		_, err = strconv.ParseInt(column.DefaultValue, 10, 64)
	case "float32", "float64":
		bits := 64
		if kind == "float32" {
			bits = 32
		}
		var value float64
		value, err = strconv.ParseFloat(column.DefaultValue, bits)
		if err == nil && (math.IsNaN(value) || math.IsInf(value, 0)) {
			err = fmt.Errorf("non-finite number")
		}
	case "bool":
		if column.DefaultValue != "true" && column.DefaultValue != "false" {
			err = fmt.Errorf("expected true or false")
		}
	}
	if err != nil {
		return fmt.Errorf("column %s defaultValue %q is not valid for %s: %w", column.ColumnName, column.DefaultValue, column.GoType, err)
	}
	return nil
}
