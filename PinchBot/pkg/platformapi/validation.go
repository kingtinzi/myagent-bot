package platformapi

import (
	"net/mail"
	"strings"
)

const InvalidEmailFormatMessage = "邮箱格式不正确，请检查后重试"

func IsLikelyValidEmailAddress(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	if strings.ContainsAny(trimmed, " \t\r\n") {
		return false
	}
	parsed, err := mail.ParseAddress(trimmed)
	if err != nil || parsed == nil {
		return false
	}
	if strings.TrimSpace(parsed.Address) != trimmed {
		return false
	}
	local, domain, ok := strings.Cut(trimmed, "@")
	if !ok || local == "" || domain == "" {
		return false
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || !strings.Contains(domain, ".") {
		return false
	}
	return true
}
