// templates/embed.go
package templates

import "embed"

//go:embed state.yaml log.yaml prompt.md review-prompt.md claude.md
var FS embed.FS
