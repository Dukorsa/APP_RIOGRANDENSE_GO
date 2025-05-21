package utils

import (
	"fmt"
	"math"     // Para Log2 na entropia
	"net/mail" // Para validação de email mais robusta
	"regexp"
	"strconv"
	"strings"
	"time" // Para uso em GenerateSecureRandomToken (exemplo)
	"unicode"
	"unicode/utf8" // Para contagem de runas (caracteres) em vez de bytes

	// Para geração de token seguro (crypto/rand)
	"crypto/rand"
	"encoding/base64"

	appErrors "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core"
	appLogger "github.com/Dukorsa/APP_RIOGRANDENSE_GO/internal/core/logger" // Para logar falhas críticas
)

// --- Validador de CNPJ ---

// IsValidCNPJ verifica se uma string de CNPJ (apenas dígitos) é válida.
// `cnpj` deve ser uma string contendo exatamente 14 dígitos numéricos.
func IsValidCNPJ(cnpj string) bool {
	if len(cnpj) != 14 {
		return false // CNPJ deve ter 14 dígitos.
	}
	// Verifica se todos os caracteres são dígitos.
	for _, r := range cnpj {
		if r < '0' || r > '9' {
			return false // Contém caracteres não numéricos.
		}
	}

	// Verifica se todos os dígitos são iguais (ex: "00000000000000"), o que é inválido.
	if allDigitsEqual(cnpj) {
		return false
	}

	// Cálculo do primeiro dígito verificador.
	sum := 0
	weights1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	for i := 0; i < 12; i++ {
		digit, _ := strconv.Atoi(string(cnpj[i])) // Erro de Atoi é ignorado pois já validamos que são dígitos.
		sum += digit * weights1[i]
	}
	remainder := sum % 11
	expectedDigit1 := 0
	if remainder >= 2 {
		expectedDigit1 = 11 - remainder
	}
	actualDigit1, _ := strconv.Atoi(string(cnpj[12]))
	if expectedDigit1 != actualDigit1 {
		return false
	}

	// Cálculo do segundo dígito verificador.
	sum = 0
	weights2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	for i := 0; i < 13; i++ {
		digit, _ := strconv.Atoi(string(cnpj[i]))
		sum += digit * weights2[i]
	}
	remainder = sum % 11
	expectedDigit2 := 0
	if remainder >= 2 {
		expectedDigit2 = 11 - remainder
	}
	actualDigit2, _ := strconv.Atoi(string(cnpj[13]))
	return expectedDigit2 == actualDigit2
}

// allDigitsEqual verifica se todos os caracteres em uma string são iguais.
func allDigitsEqual(s string) bool {
	if len(s) < 2 {
		return true // String vazia ou com um dígito é considerada "todos iguais".
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

// Domínios de e-mail comumente usados para fins temporários ou de teste, que podem ser bloqueados.
var commonDisposableEmailDomains = map[string]bool{
	"temp-mail.org":     true,
	"10minutemail.com":  true,
	"mailinator.com":    true,
	"guerrillamail.com": true,
	"throwawaymail.com": true,
	// Adicionar outros domínios conforme necessário.
	// "example.com":    true, // Manter example.com se for para testes internos.
}

// ValidateEmail verifica se um endereço de e-mail é válido.
// Retorna `nil` se válido, ou um erro (potencialmente `*appErrors.ValidationError`).
func ValidateEmail(email string) error {
	trimmedEmail := strings.TrimSpace(strings.ToLower(email)) // Normaliza para minúsculas e remove espaços.

	if trimmedEmail == "" {
		return appErrors.NewValidationError("Endereço de e-mail é obrigatório.", map[string]string{"email": "obrigatório"})
	}
	if len(trimmedEmail) > 254 { // Limite prático para endereços de e-mail.
		return appErrors.NewValidationError("Endereço de e-mail excede 254 caracteres.", map[string]string{"email": "muito longo"})
	}

	// Validação de formato usando `net/mail.ParseAddress` (mais robusto que regex simples).
	addr, err := mail.ParseAddress(trimmedEmail)
	if err != nil || addr.Address != trimmedEmail { // Checa se `ParseAddress` não alterou o e-mail (ex: removendo comentários).
		return appErrors.NewValidationError("Formato de endereço de e-mail inválido.", map[string]string{"email": "formato inválido"})
	}

	// Validação de domínio (exemplo: blacklist de domínios descartáveis).
	parts := strings.Split(trimmedEmail, "@")
	if len(parts) == 2 {
		domain := strings.ToLower(parts[1])
		if commonDisposableEmailDomains[domain] {
			return appErrors.NewValidationError("Domínio de e-mail não permitido (e-mail temporário/descartável).", map[string]string{"email": "domínio bloqueado"})
		}
		// Opcional: Verificação de MX record (requer consulta DNS, pode ser lento e sujeito a falhas de rede).
		// Atualmente desabilitado para manter o validador síncrono e rápido.
		// _, mxErr := net.LookupMX(domain)
		// if mxErr != nil {
		// 	 if dnsErr, ok := mxErr.(*net.DNSError); ok && dnsErr.IsNotFound {
		// 		 return appErrors.NewValidationError("Domínio de e-mail não encontrado (sem registros MX).", map[string]string{"email": "domínio inexistente"})
		// 	 }
		// 	 appLogger.Warnf("Erro ao verificar MX record para o domínio '%s' (e-mail: '%s'): %v", domain, trimmedEmail, mxErr)
		// 	 // Decidir se falha ou permite em caso de erro na consulta DNS. Para validação de cadastro, pode ser mais permissivo aqui.
		// }
	} else { // Não deveria acontecer se `mail.ParseAddress` passou.
		return appErrors.NewValidationError("Formato de endereço de e-mail inválido (ausente '@' ou estrutura incorreta).", map[string]string{"email": "formato inválido"})
	}

	return nil // E-mail considerado válido.
}

// --- Validador de Força de Senha ---

// PasswordStrengthResult contém os resultados da validação de força da senha.
type PasswordStrengthResult struct {
	IsValid           bool    `json:"is_valid"`            // True se todos os critérios de força forem atendidos.
	Length            bool    `json:"length_ok"`           // True se o comprimento mínimo for atendido.
	HasUppercase      bool    `json:"has_uppercase"`       // True se contém letra maiúscula.
	HasLowercase      bool    `json:"has_lowercase"`       // True se contém letra minúscula.
	HasDigit          bool    `json:"has_digit"`           // True se contém número.
	HasSpecialChar    bool    `json:"has_special_char"`    // True se contém caractere especial.
	IsNotCommon       bool    `json:"is_not_common"`       // True se NÃO for uma senha comum/fraca conhecida.
	Entropy           float64 `json:"entropy_bits"`        // Estimativa de entropia da senha em bits.
	MinLengthRequired int     `json:"min_length_required"` // Comprimento mínimo que foi exigido.
}

// GetErrorDetailsList retorna uma lista de strings descrevendo as falhas de validação de força da senha.
func (psr *PasswordStrengthResult) GetErrorDetailsList() []string {
	var details []string
	if psr.IsValid {
		return details
	} // Se válida, não há detalhes de erro.

	if !psr.Length {
		details = append(details, fmt.Sprintf("deve ter pelo menos %d caracteres", psr.MinLengthRequired))
	}
	if !psr.HasUppercase {
		details = append(details, "deve conter pelo menos uma letra maiúscula (A-Z)")
	}
	if !psr.HasLowercase {
		details = append(details, "deve conter pelo menos uma letra minúscula (a-z)")
	}
	if !psr.HasDigit {
		details = append(details, "deve conter pelo menos um número (0-9)")
	}
	if !psr.HasSpecialChar {
		details = append(details, "deve conter pelo menos um caractere especial (ex: !@#$%)")
	}
	if !psr.IsNotCommon {
		details = append(details, "senha muito comum ou fácil de adivinhar")
	}
	// Pode-se adicionar um critério de entropia mínima se desejado.
	// Ex: if psr.Entropy < 60 { details = append(details, "complexidade (entropia) insuficiente") }
	return details
}

// Lista de senhas comuns (muito pequena, para exemplo).
// Em um sistema real, usar uma biblioteca ou serviço para checagem de senhas comprometidas (ex: Pwned Passwords).
var commonPasswordsList = map[string]bool{
	"password": true, "123456": true, "qwerty": true, "admin": true, "welcome": true, "senha123": true,
	"12345678": true, "abc123": true, "password123": true, "admin123": true, "111111": true,
}

// ValidatePasswordStrength verifica a força de uma senha com base em critérios comuns.
// `minLength` é o comprimento mínimo exigido.
func ValidatePasswordStrength(password string, minLength int) PasswordStrengthResult {
	res := PasswordStrengthResult{MinLengthRequired: minLength, IsNotCommon: true} // Assume que não é comum inicialmente.

	if password == "" { // Senha vazia falha em todos os critérios.
		res.IsValid = false
		return res
	}

	// Comprimento (usando contagem de runas para caracteres Unicode).
	res.Length = utf8.RuneCountInString(password) >= minLength

	// Checagem de senha comum (case-insensitive).
	if commonPasswordsList[strings.ToLower(password)] {
		res.IsNotCommon = false
	}

	// Contagem de tipos de caracteres e cálculo de entropia.
	var charsetSize float64 = 0
	charsetsFound := 0

	for _, char := range password {
		if unicode.IsUpper(char) && !res.HasUppercase {
			res.HasUppercase = true
			charsetSize += 26
			charsetsFound++
		} else if unicode.IsLower(char) && !res.HasLowercase {
			res.HasLowercase = true
			charsetSize += 26
			charsetsFound++
		} else if unicode.IsDigit(char) && !res.HasDigit {
			res.HasDigit = true
			charsetSize += 10
			charsetsFound++
		} else if strings.ContainsRune("!@#$%^&*()_+-=[]{};':\",./<>?|\\`~", char) && !res.HasSpecialChar {
			// Define um conjunto de caracteres especiais considerados.
			res.HasSpecialChar = true
			charsetSize += 32
			charsetsFound++ // Tamanho aproximado do conjunto de especiais.
		}
	}

	// Cálculo de Entropia de Shannon (simplificado): H = L * log2(N)
	// L = comprimento da senha, N = tamanho do conjunto de caracteres possíveis.
	// Esta é uma estimativa e pode não ser perfeitamente precisa.
	if charsetSize > 0 && len(password) > 0 {
		res.Entropy = float64(utf8.RuneCountInString(password)) * math.Log2(charsetSize)
	} else {
		res.Entropy = 0
	}

	// Define a validade geral da senha com base nos critérios.
	// Uma política comum é exigir pelo menos 3 dos 4 tipos de caracteres (maiúscula, minúscula, dígito, especial).
	// Ou definir um threshold de entropia.
	// Exemplo de política: Comprimento + 3 de 4 tipos de caracteres + Não ser comum.
	criteriaMetCount := 0
	if res.HasUppercase {
		criteriaMetCount++
	}
	if res.HasLowercase {
		criteriaMetCount++
	}
	if res.HasDigit {
		criteriaMetCount++
	}
	if res.HasSpecialChar {
		criteriaMetCount++
	}

	// Política de exemplo: Comprimento OK, Não Comum, e pelo menos 3 dos 4 critérios de caracteres.
	res.IsValid = res.Length && res.IsNotCommon && (criteriaMetCount >= 3)
	// Adicionar checagem de entropia se desejar: && res.Entropy >= 60.0 (exemplo de threshold)

	return res
}

// --- Validadores para Nomes (Network, Role, Username) ---
// Regex para nomes: letras (Unicode), números, underscore, hífen.
// Ajustar {min,max} conforme necessário para cada tipo de nome.
var generalNameRegex = func(min, max int) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`^[\p{L}\d_-]{%d,%d}$`, min, max))
}
var (
	networkNameValidationRegex = generalNameRegex(3, 50)
	roleNameValidationRegex    = generalNameRegex(3, 50)
	usernameValidationRegex    = generalNameRegex(3, 50)
)

// Regex para nome do comprador: letras (Unicode), espaços, ponto, hífen.
var buyerNameValidationRegex = regexp.MustCompile(`^[\p{L}\s.-]{2,100}$`) // Mínimo de 2 caracteres.

// IsValidNetworkName valida o nome da rede.
func IsValidNetworkName(name string) bool {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return false
	}
	return networkNameValidationRegex.MatchString(trimmedName)
}

// IsValidBuyerName valida o nome do comprador.
// Adiciona uma checagem simples de ter pelo menos duas "palavras" (nomes).
func IsValidBuyerName(buyer string) bool {
	trimmedBuyer := strings.TrimSpace(buyer)
	if trimmedBuyer == "" {
		return false
	}
	if len(strings.Fields(trimmedBuyer)) < 2 { // Exige pelo menos duas palavras (ex: Nome Sobrenome).
		return false
	}
	return buyerNameValidationRegex.MatchString(trimmedBuyer)
}

// IsValidRoleName valida o nome do role (perfil).
func IsValidRoleName(name string) bool {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return false
	}
	return roleNameValidationRegex.MatchString(trimmedName)
}

// IsValidUsernameFormat valida o formato do nome de usuário (login).
func IsValidUsernameFormat(username string) bool {
	trimmedUsername := strings.TrimSpace(username)
	if trimmedUsername == "" {
		return false
	}
	return usernameValidationRegex.MatchString(trimmedUsername)
}

// --- Funções de Sanitização e Geração de Token ---

// SanitizeInput remove caracteres de controle (exceto espaços comuns como tab, newline)
// e normaliza múltiplos espaços para um único espaço.
// Esta é uma sanitização MUITO BÁSICA. Para HTML, use `html.EscapeString`.
// Para SQL, SEMPRE use prepared statements/queries parametrizadas.
func SanitizeInput(inputStr string) string {
	if inputStr == "" {
		return ""
	}

	var sb strings.Builder
	lastCharWasSpace := false
	for _, r := range inputStr {
		// Mantém caracteres imprimíveis e espaços comuns.
		if unicode.IsPrint(r) || r == '\t' || r == '\n' || r == '\r' {
			if unicode.IsSpace(r) { // Trata todos os tipos de espaço.
				if !lastCharWasSpace {
					sb.WriteRune(' ') // Adiciona um único espaço para sequências de espaços.
				}
				lastCharWasSpace = true
			} else {
				sb.WriteRune(r)
				lastCharWasSpace = false
			}
		}
		// Outros caracteres de controle são pulados (removidos).
	}
	return strings.TrimSpace(sb.String()) // Trim final para remover espaços nas extremidades.
}

// GenerateSecureRandomToken gera um token string seguro (URL-safe) com o comprimento especificado.
// Usa `crypto/rand` para bytes aleatórios e codificação Base64 URL-safe.
func GenerateSecureRandomToken(length int) string {
	// O número de bytes aleatórios necessários para gerar uma string Base64 de `length`.
	// Cada 3 bytes de dados binários se tornam 4 caracteres Base64.
	// Então, `numBytes = (length * 3) / 4` arredondado para cima.
	// Ou, mais simples, use `length` para os bytes e corte a string Base64 se for maior.
	// Para um token de 32 caracteres, 24 bytes são suficientes (24 * 4/3 = 32).
	// Para um de 64 caracteres, 48 bytes (48 * 4/3 = 64).
	// Vamos usar uma regra simples: `numBytes = (length * 6) / 8` (aproximadamente).
	// Ou, mais seguro, `numBytes = length` e depois cortar a string Base64.
	// Para garantir aleatoriedade suficiente, usamos `length` como número de bytes se for razoável.
	numBytes := length
	if numBytes < 24 {
		numBytes = 24
	} // Mínimo de bytes para um bom token.
	if numBytes > 64 {
		numBytes = 64
	} // Limita para não gerar tokens excessivamente longos por engano.

	b := make([]byte, numBytes)
	if _, err := rand.Read(b); err != nil {
		// Fallback MUITO simples se crypto/rand falhar (altamente improvável, mas crítico).
		// EM PRODUÇÃO: Trate este erro seriamente, logue como CRÍTICO, e talvez pare a aplicação
		// ou use um UUID como fallback mais robusto.
		appLogger.Errorf("Falha CRÍTICA ao gerar bytes aleatórios para token: %v. Usando fallback baseado em timestamp (NÃO SEGURO).", err)
		timestamp := time.Now().UnixNano()
		// Este fallback não é criptograficamente seguro.
		fallbackToken := fmt.Sprintf("fallback_%x_%x", timestamp, timestamp/int64(numBytes+1))
		if len(fallbackToken) > length {
			return fallbackToken[:length]
		}
		return fallbackToken
	}

	// Codifica os bytes aleatórios para Base64 URL-safe (sem padding).
	token := base64.RawURLEncoding.EncodeToString(b)

	// Corta a string do token para o comprimento desejado, se necessário.
	if len(token) > length {
		return token[:length]
	}
	// Se a string gerada for menor que `length` (improvável com `numBytes` suficiente),
	// pode-se preencher com caracteres aleatórios adicionais ou usar o token como está.
	// Por simplicidade, retorna o token gerado.
	return token
}
