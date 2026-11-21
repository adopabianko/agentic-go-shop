// Package api embeds the authoritative public HTTP contract for contract tests.
package api

import _ "embed"

//go:embed openapi.json
var Document []byte
