//go:build windows

package payload

import _ "embed"

//go:embed ls-wrapper.exe
var LSWrapper []byte
