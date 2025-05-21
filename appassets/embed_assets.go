package appassets

import "embed"

//go:embed ../assets/emails/notification.html
var EmailTemplatesFS embed.FS
