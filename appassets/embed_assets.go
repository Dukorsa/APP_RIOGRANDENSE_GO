package appassets

import "embed"

//go:embed templates/emails/*
var EmailTemplatesFS embed.FS
