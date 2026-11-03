// Package api holds the OpenAPI contract. It is written first and is the
// source the frontend generates its TypeScript types from.
package api

import _ "embed"

//go:embed openapi.yaml
var OpenAPI []byte
