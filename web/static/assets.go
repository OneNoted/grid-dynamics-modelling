package static

import "embed"

// FS contains the dashboard assets used by installed griddyn binaries.
//
//go:embed app.js index.html styles.css
var FS embed.FS
