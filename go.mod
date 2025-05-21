module github.com/Dukorsa/APP_RIOGRANDENSE_GO

go 1.21

// Diretiva replace para ajudar o Go a encontrar os pacotes gioui.org no GitHub.
// A versão/commit aqui deve ser uma válida do repositório github.com/gioui/gio.
// Usando a pseudo-versão que o `go mod tidy` tentou para `gioui.org/app` e `gioui.org/widget`.
replace gioui.org => github.com/gioui/gio v0.0.0-20240507115830-44eb715675ef

require (
	gioui.org/app v0.0.0-20240507115830-44eb715675ef
	gioui.org/font v0.0.0-20240322093531-e6be757aaf84 // Esta versão é diferente, o replace acima pode afetá-la.
	gioui.org/widget v0.0.0-20240507115830-44eb715675ef
	github.com/google/uuid v1.6.0
	github.com/joho/godotenv v1.5.1
	github.com/shopspring/decimal v1.4.0 // Adicionado explicitamente
	github.com/sirupsen/logrus v1.9.3
	github.com/xuri/excelize/v2 v2.8.1
	golang.org/x/crypto v0.23.0
	golang.org/x/exp v0.0.0-20240506185415-9bf2ced13842 // Contém icons (obsoleto)
	golang.org/x/text v0.15.0                            // Para encoding/charmap
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
	gorm.io/driver/postgres v1.5.7
	gorm.io/driver/sqlite v1.5.5
	gorm.io/gorm v1.25.9
)

// Após criar este arquivo, execute `go mod tidy` no seu terminal.
// Ele irá preencher as dependências indiretas abaixo.
// Exemplo de dependências indiretas que podem aparecer:
//
// require (
// 	github.com/davecgh/go-spew v1.1.1 // indirect
// 	github.com/jackc/pgpassfile v1.0.0 // indirect
// 	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
// 	github.com/jackc/pgx/v5 v5.5.5 // indirect
// 	github.com/jinzhu/inflection v1.0.0 // indirect
// 	github.com/jinzhu/now v1.1.5 // indirect
// 	github.com/mattn/go-sqlite3 v1.14.22 // indirect
// 	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
//	github.com/richardlehane/mscfb v1.0.4 // indirect
//	github.com/richardlehane/msoleps v1.0.3 // indirect
// 	golang.org/x/image v0.15.0 // indirect // gio dependency
// 	golang.org/x/sys v0.20.0 // indirect
//	github.com/xuri/efp v0.0.0-20231025114911-3dfd139d6ca3 // indirect
//	github.com/xuri/nfp v0.0.0-20230919160717-d98642af3f0b // indirect
// )