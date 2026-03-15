package payments

import "strings"

const DefaultSiteName = "PinchBot"

func NormalizeSiteName(siteName string) string {
	siteName = strings.TrimSpace(siteName)
	if siteName == "" {
		return DefaultSiteName
	}
	return siteName
}

func RechargeDisplayName(siteName string) string {
	return NormalizeSiteName(siteName) + " Recharge"
}
