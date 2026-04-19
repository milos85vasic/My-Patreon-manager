package database

import "embed"

// embeddedMigrations holds every versioned migration .sql file shipped
// with the binary. Read by Migrator via the NewMigrator() helpers on the
// driver structs.
//
//go:embed migrations/*.sql
var embeddedMigrations embed.FS
