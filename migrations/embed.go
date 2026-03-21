// Package migrations provides embedded SQL schema migration files.
// Based on: docs/data-model.md §9 (Data Migration and Versioning)
// 3GPP: TS 29.505 — Complete UDM subscriber data schema
package migrations

import "embed"

// FS contains the embedded SQL migration files for the UDM database schema.
// Migration files follow the naming convention: NNNNNN_description.{up|down}.sql
// Use with internal/db.ParseMigrations() to load migrations.
//
//go:embed *.sql
var FS embed.FS
