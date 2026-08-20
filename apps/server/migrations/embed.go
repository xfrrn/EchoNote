package migrations

import "embed"

// Files contains the versioned SQL migrations used by the migrate command.
//
//go:embed *.sql
var Files embed.FS
