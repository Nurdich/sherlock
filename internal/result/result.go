// Copyright (c) 2019 Sherlock Project
// SPDX-License-Identifier: MIT

// Package result records the outcome of a username query.
package result

// Status describes whether a username appears to exist on a site.
type Status string

const (
	// Claimed means the username was detected.
	Claimed Status = "Claimed"
	// Available means the username was not detected.
	Available Status = "Available"
	// Unknown means an error occurred while trying to detect the username.
	Unknown Status = "Unknown"
	// Illegal means the username is not allowable for the site.
	Illegal Status = "Illegal"
	// WAF means the request was blocked by a web application firewall.
	WAF Status = "WAF"
)

// String returns the status label shown to users and written to reports.
func (s Status) String() string { return string(s) }

// Result is the outcome of checking one username on one site.
type Result struct {
	Username    string
	SiteName    string
	SiteURLUser string
	Status      Status
	// QueryTime is the probe duration in seconds. Nil when it was not measured.
	QueryTime *float64
	// Context is extra information, such as an error category. Nil when absent.
	Context *string
}

// String formats the status, appending context when it is present.
func (r Result) String() string {
	status := r.Status.String()
	if r.Context != nil {
		status += " (" + *r.Context + ")"
	}
	return status
}
