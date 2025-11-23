package main

import (
	"fmt"
	"runtime"
)

// Version information (set by GoReleaser at build time)
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "unknown"
)

// VersionInfo contains version information
type VersionInfo struct {
	Version   string
	Commit    string
	Date      string
	BuiltBy   string
	GoVersion string
}

// GetVersion returns the version information
func GetVersion() VersionInfo {
	return VersionInfo{
		Version:   version,
		Commit:    commit,
		Date:      date,
		BuiltBy:   builtBy,
		GoVersion: runtime.Version(),
	}
}

// String returns a formatted version string
func (v VersionInfo) String() string {
	return fmt.Sprintf("DuraGraph %s\nCommit: %s\nBuilt: %s by %s\nGo: %s",
		v.Version, v.Commit, v.Date, v.BuiltBy, v.GoVersion)
}

// ShortVersion returns a short version string
func (v VersionInfo) ShortVersion() string {
	return v.Version
}
