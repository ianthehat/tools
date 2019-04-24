package links

import (
	"fmt" //@link(re`".*"`,"https://godoc.org/fmt")

	"golang.org/x/tools/internal/lsp/protocol" //@link(re`".*"`,"https://godoc.org/golang.org/x/tools/internal/lsp/protocol")
)

var (
	_ fmt.Formatter
	_ protocol.Client
)
