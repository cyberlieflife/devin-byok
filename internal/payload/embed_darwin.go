//go:build darwin

package payload

import _ "embed"

//go:embed ls-wrapper
var LSWrapper []byte
