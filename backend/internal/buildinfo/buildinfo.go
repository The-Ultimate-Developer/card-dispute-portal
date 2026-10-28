// Package buildinfo holds values stamped into the binary at build time via -ldflags.
package buildinfo

// Set by: go build -ldflags "-X .../buildinfo.Version=<sha> -X .../buildinfo.BuildTime=<time>"
var (
	Version   = "dev"
	BuildTime = "unknown"
)
