module github.com/Dukorsa/APP_RIOGRANDENSE_GO

go 1.23.0

toolchain go1.24.2

// Diretiva replace para forçar o Go a usar o repositório GitHub para todos os pacotes gioui.org,
// utilizando uma versão consistente.

require (
	// As dependências do Gio serão adicionadas aqui pelo 'go mod tidy'
	// com a versão especificada na diretiva 'replace'.
	// Deixe esta seção vazia de entradas 'gioui.org/...' inicialmente.
	github.com/gioui/gio v0.8.0
	github.com/google/uuid v1.6.0
	github.com/joho/godotenv v1.5.1
	github.com/shopspring/decimal v1.4.0
	github.com/sirupsen/logrus v1.9.3
	github.com/xuri/excelize/v2 v2.8.1
	golang.org/x/crypto v0.23.0
	golang.org/x/exp v0.0.0-20240506185415-9bf2ced13842 // Para icons (ainda pode ser problemático se o pacote for removido)
	golang.org/x/text v0.15.0
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
	gorm.io/driver/postgres v1.5.7
	gorm.io/driver/sqlite v1.5.5
	gorm.io/gorm v1.25.9
)

require golang.org/x/exp/shiny v0.0.0-20250506013437-ce4c2cf36ca6 // indirect

// As dependências indiretas e as versões corretas dos pacotes Gio
// serão preenchidas aqui pelo comando 'go mod tidy'.
