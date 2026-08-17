//go:build windows

package payload

import _ "embed"

//go:embed ls-wrapper.exe
var LSWrapper []byte

//go:embed devin-wrapper.exe
var DevinWrapper []byte
