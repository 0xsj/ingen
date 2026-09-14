package version

// These values are replaced at release build time with Go's -ldflags -X.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)
