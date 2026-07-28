package template

import "embed"

// Assets is the feature-based WebUI tree (layout + shared + features).
//
//go:embed all:assets
var Assets embed.FS
