// No arquivo: APP_RIOGRANDENSE_GO/appassets/embed.go
package appassets

import "embed"

//go:embed ../assets/emails/*.html ../assets/emails/*.txt
var EmailTemplatesFS embed.FS