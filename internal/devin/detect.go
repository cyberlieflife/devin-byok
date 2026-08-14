package devin

import "devin-byok/internal/platform"

func DetectDataDirs() []string {
	return platform.DevinDataDirs()
}
