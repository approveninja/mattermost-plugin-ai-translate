package translate

import "strings"

// Language is a target language offered in the translate dropdown.
type Language struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// DefaultLanguage is the target used until the user picks another.
const DefaultLanguage = "EN"

// Languages is the fixed, ordered list shown in the UI.
var Languages = []Language{
	{"EN", "English"},
	{"UA", "Ukrainian"},
	{"RU", "Russian"},
	{"PL", "Polish"},
	{"ES", "Spanish"},
	{"FR", "French"},
	{"DE", "German"},
	{"ZH", "Chinese"},
	{"IT", "Italian"},
	{"PT", "Portuguese"},
	{"JA", "Japanese"},
	{"TR", "Turkish"},
}

// LanguageName returns the English display name for a language code (case-insensitive).
func LanguageName(code string) (string, bool) {
	up := strings.ToUpper(strings.TrimSpace(code))
	for _, l := range Languages {
		if l.Code == up {
			return l.Name, true
		}
	}
	return "", false
}

// IsSupported reports whether code is in the supported set.
func IsSupported(code string) bool {
	_, ok := LanguageName(code)
	return ok
}
