module github.com/Dukorsa/APP_RIOGRANDENSE_GO // Substitua por seu_usuario/seu_repo_name

go 1.21 // Ou a versão mais recente do Go que você está usando

require (
	github.com/go-playground/validator/v10 v10.16.0 // Para validação de structs
	github.com/google/uuid v1.4.0                   // Para IDs UUID
	github.com/joho/godotenv v1.5.1                 // Para carregar arquivos .env
	github.com/lib/pq v1.10.9                       // Driver PostgreSQL
	// github.com/mattn/go-sqlite3 v1.14.17         // Driver SQLite3 (descomente se usar)
	github.com/sirupsen/logrus v1.9.3                 // Para logging estruturado
	golang.org/x/crypto v0.15.0                     // Para bcrypt (hash de senhas)
	gopkg.in/natefinch/lumberjack.v2 v2.2.1         // Para rotação de arquivos de log
	github.com/xuri/excelize/v2 v2.8.0              // Para exportação XLSX (adicionado agora)
    golang.org/x/text v0.14.0                       // Para normalização de texto, title casing (dependência de outras libs)

	// Se você decidir usar um ORM como GORM:
	gorm.io/driver/postgres v1.5.4              // Driver GORM para PostgreSQL
	gorm.io/driver/sqlite v1.5.4                // Driver GORM para SQLite
	gorm.io/gorm v1.25.5                        // O ORM GORM

	// Se você decidir usar sqlx em vez de um ORM completo:
	// github.com/jmoiron/sqlx v1.3.5
)

// 'indirect' dependencies são gerenciadas automaticamente pelo Go.
// Você pode precisar de 'replace' directives se estiver usando forks
// ou versões locais de bibliotecas durante o desenvolvimento.
// Exemplo:
// replace gioui.org => ../gioui // Se você tiver um clone local do Gio