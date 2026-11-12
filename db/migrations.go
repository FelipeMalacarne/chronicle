// Package db embeds the goose migrations of db/migrations, so `chronicle
// migrate` needs no files next to the binary (the image is one binary).
package db

import "embed"

// Migrations holds migrations/*.sql.
//
//go:embed migrations/*.sql
var Migrations embed.FS
