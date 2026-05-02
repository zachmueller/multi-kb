package assets

import "embed"

//go:embed *.html *.css *.js
var Assets embed.FS
