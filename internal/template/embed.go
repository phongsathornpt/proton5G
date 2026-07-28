package template

import "embed"

//go:embed index.html style.css app.js
var Files embed.FS
