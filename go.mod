module github.com/Dukorsa/APP_RIOGRANDENSE_GO

go 1.21 // Especifique a versão do Go que você está usando (ex: 1.21, 1.22)

require (
	gioui.org/app v0.0.0-20240507115830-44eb715675ef // Use a versão mais recente compatível
	gioui.org/font v0.0.0-20240322093531-e6be757aaf84 // Use a versão mais recente compatível
	gioui.org/widget v0.0.0-20240507115830-44eb715675ef // Use a versão mais recente compatível
	github.com/google/uuid v1.6.0
	github.com/joho/godotenv v1.5.1
	github.com/sirupsen/logrus v1.9.3
	github.com/xuri/excelize/v2 v2.8.1
	golang.org/x/crypto v0.23.0
	golang.org/x/exp v0.0.0-20240506185415-9bf2ced13842 // Contém shiny/materialdesign/icons (ATENÇÃO: obsoleto)
	golang.org/x/text v0.15.0                            // Para encoding/charmap e transform
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
	gorm.io/driver/postgres v1.5.7
	gorm.io/driver/sqlite v1.5.5
	gorm.io/gorm v1.25.9
)