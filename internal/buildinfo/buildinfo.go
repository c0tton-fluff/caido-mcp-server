// Package buildinfo resolves the binary version across build methods.
//
// Release builds inject the version via -ldflags "-X ...buildinfo.version=vX.Y.Z"
// (see scripts/build.sh). Builds produced by "go install ...@vX.Y.Z" cannot set
// ldflags, so we fall back to the module version recorded by the Go toolchain
// via runtime/debug.ReadBuildInfo. Plain "go build" with neither yields "dev".
package buildinfo

import "runtime/debug"

// version is overridden at release build time via -ldflags.
var version = "dev"

// Version returns the resolved binary version.
//
// Precedence: an ldflag-injected version wins; otherwise the module version
// from the build info (populated by "go install pkg@version") is used; failing
// both, it reports "dev".
func Version() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return version
}
