// Copyright (c) 2019 Sherlock Project
// SPDX-License-Identifier: MIT

// Package version holds the user-visible Sherlock version.
package version

// Version is the Sherlock release shown by --version.
// It can be overridden at link time with -X.
var Version = "0.16.2"

const (
	// ShortName is the program name used in version output.
	ShortName = "Sherlock"
	// LongName is the description used in help output.
	LongName = "Sherlock: Find Usernames Across Social Networks"
)

// UserAgent is sent on manifest, exclusion, and update requests.
func UserAgent() string {
	return "sherlock/" + Version
}
