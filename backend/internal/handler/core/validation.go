package core

import "regexp"

var ValidName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

var ProtectedDatabases = map[string]bool{
	"template0": true,
	"template1": true,
	"postgres":  true,
	"pgmanager": true,
}
