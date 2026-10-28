// Package migrations embeds the SQL files into the binary, so the image
// carries its own schema and needs no extra files at runtime.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
