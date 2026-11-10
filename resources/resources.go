// Copyright (c) 2019 Sherlock Project
// SPDX-License-Identifier: MIT
//
// Go port of https://github.com/sherlock-project/sherlock

// Package resources embeds the Sherlock site manifest shipped with this fork.
package resources

import _ "embed"

// DataJSON is the site manifest (the same data set as the original project).
//
//go:embed data.json
var DataJSON []byte

// DataSchema is the JSON Schema for DataJSON.
//
//go:embed data.schema.json
var DataSchema []byte
