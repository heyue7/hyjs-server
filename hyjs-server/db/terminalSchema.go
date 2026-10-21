package db

import "embed"

//go:embed sql/240_terminal_*.sql
var terminalSQL embed.FS

// TerminalSchemaSQL returns the checked-in durable terminal table definition.
func TerminalSchemaSQL(name string) ([]byte, error) {
	return terminalSQL.ReadFile("sql/240_" + name + ".sql")
}
