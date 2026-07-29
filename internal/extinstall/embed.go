package extinstall

import "embed"

//go:embed all:extfiles
var ExtFS embed.FS

// ExtRoot is the embed root directory name.
const ExtRoot = "extfiles"
