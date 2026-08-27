// Package buildinfo exposes metadata embedded into the service binary.
package buildinfo

import (
	"os"
	"strings"
)

// Version is set from the repository VERSION file through Go linker flags.
var Version string

// Resolve returns the embedded version. Reading VERSION is a local go-run
// fallback; deployed binaries always receive Version during the image build.
func Resolve(fallback string) string {
	if version := strings.TrimSpace(Version); version != "" {
		return version
	}
	if contents, err := os.ReadFile("VERSION"); err == nil {
		if version := strings.TrimSpace(string(contents)); version != "" {
			return version
		}
	}
	return strings.TrimSpace(fallback)
}
