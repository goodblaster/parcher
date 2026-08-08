package tesseract

import (
	"regexp"
	"strings"
)

// LanguageMap maps ISO 639-1 language codes and their regional/script variations to Tesseract language codes
var LanguageMap = map[string]string{
	"af":      "afr",     // Afrikaans
	"am":      "amh",     // Amharic
	"ar":      "ara",     // Arabic
	"as":      "asm",     // Assamese
	"az":      "aze",     // Azerbaijani
	"be":      "bel",     // Belarusian
	"bg":      "bul",     // Bulgarian
	"bn":      "ben",     // Bengali
	"bs":      "bos",     // Bosnian
	"ca":      "cat",     // Catalan
	"cs":      "ces",     // Czech
	"cy":      "cym",     // Welsh
	"da":      "dan",     // Danish
	"de":      "deu",     // German
	"el":      "ell",     // Greek
	"en":      "eng",     // English
	"en-us":   "eng",     // English (United States)
	"en-gb":   "eng",     // English (United Kingdom)
	"eo":      "epo",     // Esperanto
	"es":      "spa",     // Spanish
	"et":      "est",     // Estonian
	"eu":      "eus",     // Basque
	"fa":      "fas",     // Persian
	"fi":      "fin",     // Finnish
	"fil":     "fil",     // Filipino
	"fr":      "fra",     // French
	"ga":      "gle",     // Irish
	"gl":      "glg",     // Galician
	"gu":      "guj",     // Gujarati
	"he":      "heb",     // Hebrew
	"hi":      "hin",     // Hindi
	"hr":      "hrv",     // Croatian
	"hu":      "hun",     // Hungarian
	"hy":      "hye",     // Armenian
	"id":      "ind",     // Indonesian
	"is":      "isl",     // Icelandic
	"it":      "ita",     // Italian
	"ja":      "jpn",     // Japanese
	"jv":      "jav",     // Javanese
	"ka":      "kat",     // Georgian
	"kk":      "kaz",     // Kazakh
	"km":      "khm",     // Khmer
	"kn":      "kan",     // Kannada
	"ko":      "kor",     // Korean
	"lo":      "lao",     // Lao
	"lt":      "lit",     // Lithuanian
	"lv":      "lav",     // Latvian
	"mk":      "mkd",     // Macedonian
	"ml":      "mal",     // Malayalam
	"mn":      "mon",     // Mongolian
	"mr":      "mar",     // Marathi
	"ms":      "msa",     // Malay
	"my":      "mya",     // Burmese
	"ne":      "nep",     // Nepali
	"nl":      "nld",     // Dutch
	"no":      "nor",     // Norwegian
	"or":      "ori",     // Odia (Oriya)
	"pa":      "pan",     // Punjabi
	"pl":      "pol",     // Polish
	"ps":      "pus",     // Pashto
	"pt":      "por",     // Portuguese
	"pt-br":   "por",     // Portuguese (Brazil)
	"pt-pt":   "por",     // Portuguese (Portugal)
	"ro":      "ron",     // Romanian
	"ru":      "rus",     // Russian
	"si":      "sin",     // Sinhala
	"sk":      "slk",     // Slovak
	"sl":      "slv",     // Slovenian
	"sq":      "sqi",     // Albanian
	"sr":      "srp",     // Serbian
	"su":      "sun",     // Sundanese
	"sv":      "swe",     // Swedish
	"sw":      "swa",     // Swahili
	"ta":      "tam",     // Tamil
	"te":      "tel",     // Telugu
	"th":      "tha",     // Thai
	"tr":      "tur",     // Turkish
	"uk":      "ukr",     // Ukrainian
	"ur":      "urd",     // Urdu
	"uz":      "uzb",     // Uzbek
	"vi":      "vie",     // Vietnamese
	"zh":      "chi_sim", // Chinese (default to Simplified)
	"zh-hans": "chi_sim", // Chinese Simplified
	"zh-cn":   "chi_sim", // Chinese (China)
	"zh-sg":   "chi_sim", // Chinese (Singapore)
	"zh-hant": "chi_tra", // Chinese Traditional
	"zh-hk":   "chi_tra", // Chinese (Hong Kong)
	"zh-mo":   "chi_tra", // Chinese (Macau)
	"zh-tw":   "chi_tra", // Chinese (Taiwan)
}

// NormalizeLangCode extracts the primary language and optional region/script from a website language code
func NormalizeLangCode(lang string) string {
	lang = strings.ToLower(lang)
	re := regexp.MustCompile(`^([a-z]{2,3})(?:[-_](hans|hant|[a-z]{2}))?$`)
	matches := re.FindStringSubmatch(lang)

	if matches != nil {
		primary := matches[1]
		var variant string
		if len(matches) > 2 {
			variant = matches[2]
		}

		// Attempt to find a match for the full language-variant code
		if variant != "" {
			combined := primary + "-" + variant
			if tessLang, exists := LanguageMap[combined]; exists {
				return tessLang
			}
		}

		// Fallback to the primary language code
		if tessLang, exists := LanguageMap[primary]; exists {
			return tessLang
		}
	}

	return "" // No match found
}
