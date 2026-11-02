package constants

import (
	_ "embed"
	"strings"
)

// License is the license text embedded in the binary (`futrou license`).
//
//go:embed LICENSE.txt
var licenseText string

var License = strings.TrimSpace(licenseText)
