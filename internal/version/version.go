package version

// Version is the release version (override with -ldflags).
var Version = "1.2.4"

// BuildTime can be injected via ldflags.
var BuildTime = "dev"
