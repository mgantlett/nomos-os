package cockpit

import "embed"

// Assets embeds the web dashboard static HTML, CSS, and JS frontend files.
//
//go:embed ui/*
var Assets embed.FS
