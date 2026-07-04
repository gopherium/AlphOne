// SPDX-License-Identifier: AGPL-3.0-or-later

// Package postgres provides the PostgreSQL data layer for the CRM core.
package postgres

import "embed"

// Migrations holds the core schema migration files applied by goose.
//
//go:embed migrations/*.sql
var Migrations embed.FS
