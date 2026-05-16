package buildinfo

import "fmt"

// Version and Commit can be overridden at build time with -ldflags -X.
var (
	Version = "dev"
	Commit  = ""
)

// VersionString returns a compact printable version for command-line tools.
func VersionString(binary string) string {
	if Commit == "" {
		return fmt.Sprintf("%s %s", binary, Version)
	}
	return fmt.Sprintf("%s %s (%s)", binary, Version, Commit)
}
