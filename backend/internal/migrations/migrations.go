// Package migrations embeds the database schema so the binary can apply it
// at startup without needing a separate migration tool for this small
// project (v0 convenience — the NestJS reference uses TypeORM's
// synchronize:true for the same reason).
package migrations

import _ "embed"

//go:embed schema.sql
var Schema string
