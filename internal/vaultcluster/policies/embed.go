package policies

import "embed"

//go:embed templates/*.hcl.tpl
var Templates embed.FS
