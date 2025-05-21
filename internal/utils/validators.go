package utils

import (
	"fmt"
	"net" // Para validação de IP (se necessário)
	"net/mail" // Para validação de email mais robusta
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8" // Para contagem de runas (caracteres) em vez de bytes

	// "github.com/go-playground/validator/v10" // Importar se usar para structs

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/errors" // Para retornar erros customizados
	// appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger"
)

// --- Validador de CNPJ ---

// IsValidCNPJ verifica se uma string de CNPJ (apenas dígitos) é válida.
func IsValidCNPJ(cnpj string) bool {
	if len(cnpj) != 14 {
		return false
	}
	// Verifica se todos os dígitos são iguais (ex: "00000000000000")
	if均DigitsEqual(cnpj) {
		return false
	}

	// Cálculo do primeiro dígito verificador
	sum := 0
	weights1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	for i := 0; i < 12; i++ {
		digit, _ := strconv.Atoi(string(cnpj[i]))
		sum += digit * weights1[i]
	}
	remainder := sum % 11
	digit1 := 0
	if remainder >= 2 {
		digit1 = 11 - remainder
	}
	if strconv.Itoa(digit1) != string(cnpj[12]) {
		return false
	}

	// Cálculo do segundo dígito verificador
	sum = 0
	weights2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	for i := 0; i < 13; i++ {
		digit, _ := strconv.Atoi(string(cnpj[i]))
		sum += digit * weights2[i]
	}
	remainder = sum % 11
	digit2 := 0
	if remainder >= 2 {
		digit2 = 11 - remainder
	}
	return strconv.Itoa(digit2) == string(cnpj[13])
}

//均DigitsEqual verifica se todos os caracteres em uma string são iguais.
func均DigitsEqual(s string) bool {
	if len(s) < 2 {
		return true // Ou false, dependendo da definição
	}
	first := s[0]
	for i := 1; i < len(s); i++ {
		if s[i] != first {
			return false
		}
	}
	return true
}

// --- Validador de E-mail ---
// Regex mais simples, para validação mais robusta, use mail.ParseAddress.
var emailRegexSimple = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
var domainBlacklist = map[string]bool{
	"temp-mail.org": true,
	"example.com":   true,
	"mailinator.com":true,
}

// ValidateEmail verifica se um e-mail é válido.
// Retorna nil se válido, ou um erro do tipo appErrors.ValidationError.
func ValidateEmail(email string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return appErrors.NewValidationError("E-mail é obrigatório.", map[string]string{"email": "obrigatório"})
	}
	if len(email) > 254 {
		return appErrors.NewValidationError("E-mail excede 254 caracteres.", map[string]string{"email": "muito longo"})
	}

	// Validação de formato usando net/mail (mais robusto que regex simples)
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email { // Checa se ParseAddress não alterou o email (ex: removendo comentários)
		return appErrors.NewValidationError("Formato de e-mail inválido.", map[string]string{"email": "formato inválido"})
	}

	// Validação de domínio (exemplo de blacklist)
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		domain := strings.ToLower(parts[1])
		if domainBlacklist[domain] {
			return appErrors.NewValidationError("Domínio de e-mail não permitido.", map[string]string{"email": "domínio bloqueado"})
		}
		// Opcional: Verificação de MX record (requer consulta DNS, pode ser lento)
		// _, mxErr := net.LookupMX(domain)
		// if mxErr != nil {
		// 	 if dnsErr, ok := mxErr.(*net.DNSError); ok && dnsErr.IsNotFound {
		// 		 return appErrors.NewValidationError("Domínio de e-mail não encontrado (sem MX records).", map[string]string{"email": "domínio inexistente"})
		// 	 }
		// 	 appLogger.Warnf("Erro ao verificar MX record para %s: %v", domain, mxErr)
		// 	 // Decidir se falha ou permite em caso de erro na consulta DNS
		// }
	} else { // Não deveria acontecer se mail.ParseAddress passou
		return appErrors.NewValidationError("Formato de e-mail inválido (sem @).", map[string]string{"email": "formato inválido"})
	}

	return nil
}

// --- Validador de Força de Senha ---

// PasswordStrengthResult contém os resultados da validação de força da senha.
type PasswordStrengthResult struct {
	IsValid          bool    `json:"is_valid"`
	Length           bool    `json:"length"`
	Uppercase        bool    `json:"uppercase"`
	Lowercase        bool    `json:"lowercase"`
	Digit            bool    `json:"digit"`
	SpecialChar      bool    `json:"special_char"`
	NotCommonPassword bool    `json:"not_common_password"` // True se NÃO for comum
	Entropy          float64 `json:"entropy"`
	MinLengthRequired int    `json:"min_length_required"`
}

// GetErrorDetailsList retorna uma lista de strings descrevendo as falhas de validação.
func (psr *PasswordStrengthResult) GetErrorDetailsList() []string {
	var details []string
	if !psr.IsValid { // Só adiciona detalhes se a senha geral for inválida
		if !psr.Length {
			details = append(details, fmt.Sprintf("comprimento mínimo de %d caracteres", psr.MinLengthRequired))
		}
		if !psr.Uppercase {
			details = append(details, "letra maiúscula")
		}
		if !psr.Lowercase {
			details = append(details, "letra minúscula")
		}
		if !psr.Digit {
			details = append(details, "número")
		}
		if !psr.SpecialChar {
			details = append(details, "caractere especial")
		}
		if !psr.NotCommonPassword {
			details = append(details, "senha muito comum")
		}
		// Poderia adicionar um critério de entropia mínima se desejado
		// if psr.Entropy < minEntropyRequired { details = append(details, "entropia insuficiente") }
	}
	return details
}


var commonPasswords = map[string]bool{ // Usar map para lookup O(1)
	"password": true, "123456": true, "qwerty": true, "admin": true, "welcome": true,
	"senha123": true, "12345678": true, "abc123": true, "password123": true,
	"admin123": true, "111111": true, "123123": true, "dragon": true, "monkey": true,
}

// ValidatePasswordStrength verifica a força de uma senha.
func ValidatePasswordStrength(password string, minLength int) PasswordStrengthResult {
	res := PasswordStrengthResult{MinLengthRequired: minLength}

	if password == "" { // Senha vazia falha em tudo
		res.IsValid = false
		return res
	}

	res.Length = utf8.RuneCountInString(password) >= minLength // Usa RuneCount para caracteres Unicode
	res.NotCommonPassword = !commonPasswords[strings.ToLower(password)]

	var charsetSize float64
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			res.Uppercase = true
		case unicode.IsLower(char):
			res.Lowercase = true
		case unicode.IsDigit(char):
			res.Digit = true
		// Definir caracteres especiais explicitamente, pois unicode.IsSymbol/IsPunct é muito amplo
		case strings.ContainsRune("!@#$%^&*(),.?\":{}|<>~`[]\\;',./_+-=", char):
			res.SpecialChar = true
		}
	}

	if res.Lowercase { charsetSize += 26 }
	if res.Uppercase { charsetSize += 26 }
	if res.Digit { charsetSize += 10 }
	if res.SpecialChar { charsetSize += 32 } // Aproximação do número de especiais comuns

	if charsetSize > 0 && len(password) > 0 {
		// Entropia de Shannon: H = L * log2(N)
		// Onde L é o comprimento da senha e N é o tamanho do conjunto de caracteres possíveis.
		// Esta é uma simplificação, pois assume que os caracteres são independentes e uniformemente distribuídos.
		// Mas é uma métrica comum.
		// res.Entropy = float64(utf8.RuneCountInString(password)) * math.Log2(charsetSize)

		// Para ser mais simples, podemos usar o cálculo do Python, que parecia ser um score e não entropia real.
		// Vamos calcular um "score de diversidade"
		diversityScore := 0
		if res.Lowercase { diversityScore++ }
		if res.Uppercase { diversityScore++ }
		if res.Digit { diversityScore++ }
		if res.SpecialChar { diversityScore++ }
		// Atribuir entropia com base no score de diversidade e comprimento (muito simplificado)
		res.Entropy = float64(utf8.RuneCountInString(password) * diversityScore * 5) // Ajustar multiplicador
	} else {
		res.Entropy = 0
	}
	
	// A versão Python usava uma entropia mínima de 60 bits.
	// Vamos definir a validade com base nos critérios individuais por enquanto.
	res.IsValid = res.Length && res.Uppercase && res.Lowercase && res.Digit && res.SpecialChar && res.NotCommonPassword
	// Adicionar checagem de entropia se quiser: && res.Entropy >= 60.0

	return res
}


// --- Validadores para Network ---
// Regex permite letras (incluindo acentuadas com \p{L}), números, espaços, hífen, underscore.
var networkNameRegex = regexp.MustCompile(`^[\p{L}\d\s_-]{3,50}$`)
// Permite letras (incluindo acentuadas), espaços, ponto, hífen.
var buyerNameRegex = regexp.MustCompile(`^[\p{L}\s.-]{5,100}$`)

// IsValidNetworkName valida o nome da rede.
func IsValidNetworkName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" { return false }
	return networkNameRegex.MatchString(name)
}

// IsValidBuyerName valida o nome do comprador.
func IsValidBuyerName(buyer string) bool {
	buyer = strings.TrimSpace(buyer)
	if buyer == "" { return false }
	// A validação de "pelo menos nome e sobrenome" do Python é mais complexa (contar palavras).
	// Aqui, apenas o formato regex.
	if len(strings.Fields(buyer)) < 2 { // Checa se tem pelo menos duas "palavras"
		return false
	}
	return buyerNameRegex.MatchString(buyer)
}

// --- Validador para Nome de Role ---
var roleNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{3,50}$`)

// IsValidRoleName valida o nome do role.
func IsValidRoleName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" { return false }
	return roleNameRegex.MatchString(name)
}

// --- Validador para Username ---
var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,50}$`)

// IsValidUsernameFormat valida o formato do nome de usuário.
func IsValidUsernameFormat(username string) bool {
	username = strings.TrimSpace(username)
	if username == "" { return false }
	return usernameRegex.MatchString(username)
}


// --- Funções de Sanitização (Exemplo) ---
// SQL_KEYWORDS do Python é extenso. Para Go, uma abordagem mais simples ou
// o uso de prepared statements (parâmetros de query) é a defesa primária contra SQL Injection.

// SanitizeInput remove caracteres de controle (exceto espaços comuns) e stripping básico.
// Esta é uma sanitização MUITO básica. Para HTML, use html.EscapeString.
// Para SQL, use SEMPRE prepared statements/queries parametrizadas.
func SanitizeInput(inputStr string) string {
	if inputStr == "" {
		return ""
	}
	// Remove caracteres de controle, exceto tab, newline, carriage return
	// e substitui múltiplos espaços por um único.
	var sb strings.Builder
	lastWasSpace := false
	for _, r := range inputStr {
		if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' {
			continue // Pula outros caracteres de controle
		}
		if unicode.IsSpace(r) {
			if !lastWasSpace {
				sb.WriteRune(' ') // Adiciona um único espaço
				lastWasSpace = true
			}
		} else {
			sb.WriteRune(r)
			lastWasSpace = false
		}
	}
	return strings.TrimSpace(sb.String())
}

// GenerateSecureRandomToken gera um token string seguro (URL-safe).
func GenerateSecureRandomToken(length int) string {
    // Para gerar um token string, podemos usar crypto/rand para bytes e depois codificar.
    // Exemplo: base64 URL-safe
    // import "crypto/rand"
    // import "encoding/base64"
    
    numBytes := length // Cada byte do rand pode gerar ~1.3 caracteres base64
    if length < 24 { numBytes = 24 } // Garante bytes suficientes para um token razoável

    b := make([]byte, numBytes)
    if _, err := rand.Read(b); err != nil {
        // Fallback MUITO simples se crypto/rand falhar (altamente improvável)
        // EM PRODUÇÃO: Trate este erro seriamente ou use um UUID.
        appLogger.Errorf("Falha crítica ao gerar bytes aleatórios para token: %v. Usando fallback.", err)
        timestamp := time.Now().UnixNano()
        fallback := fmt.Sprintf("fallback_%x_%x", timestamp, timestamp/int64(length+1))
        if len(fallback) > length { return fallback[:length] }
        return fallback
    }
    token := base64.URLEncoding.EncodeToString(b)
    // Remover padding '=' e cortar no tamanho desejado (base64 pode ser maior)
    token = strings.ReplaceAll(token, "=", "")
    if len(token) > length {
        return token[:length]
    }
    // Se for menor (improvável com numBytes suficiente), pode precisar preencher ou usar o token inteiro.
    return token
}

// TODO: Implementar ou integrar com github.com/go-playground/validator/v10
// para registrar validadores customizados (cnpj, password_strength, etc.)
// e usá-los com tags em structs de input (ex: UserCreate, NetworkCreate).