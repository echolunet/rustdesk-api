package web

import "embed"

// AdminFS embeds the built-in Web Admin Console static files.
//
//go:embed admin/*
var AdminFS embed.FS
