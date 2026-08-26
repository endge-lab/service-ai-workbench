package migrations

import "embed"

// FS contains ordered Workbench database migrations.
//
//go:embed *.sql
var FS embed.FS
