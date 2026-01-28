// Package docs provides embedded documentation files.
package docs

import (
	_ "embed"
)

//go:embed README.md
var Readme []byte
