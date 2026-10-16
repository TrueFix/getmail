package main

import _ "embed"

// VERSION is the single source of truth for the application release number.
//
//go:embed VERSION
var version string
