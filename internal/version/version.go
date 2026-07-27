package version

// Version is the release version (override with -ldflags).
var Version = "1.2.8"

// BuildTime can be injected via ldflags.
var BuildTime = "dev"
