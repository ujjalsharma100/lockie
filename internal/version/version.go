package version

// Version, Commit, and Date are set at build time via -ldflags.
var (
	Version = "0.0.0-dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns the user-facing version line (e.g. "lockie 0.0.0-dev").
func String() string {
	return "lockie " + Version
}
