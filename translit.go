package mewsync

import "strings"

// Transliterate romanizes lyric text for scripts that have a well-defined,
// deterministic table or algorithm: Cyrillic and Greek go through a
// per-character mapping, Hangul goes through the standard jamo decomposition
// and Revised Romanization mapping. Everything else, including Han
// characters (Chinese hanzi, Japanese kanji), passes through unchanged:
// there's no small deterministic algorithm for those, converting them to
// their spoken reading needs a pronunciation dictionary (and for kanji,
// context to disambiguate readings), which is out of scope here.
func Transliterate(text string) string {
	var out strings.Builder
	for _, r := range text {
		switch {
		case r >= 0xAC00 && r <= 0xD7A3:
			out.WriteString(transliterateHangulSyllable(r))
		case r >= 0x0400 && r <= 0x04FF:
			if s, ok := cyrillicTable[r]; ok {
				out.WriteString(s)
			} else {
				out.WriteRune(r)
			}
		case r >= 0x0370 && r <= 0x03FF:
			if s, ok := greekTable[r]; ok {
				out.WriteString(s)
			} else {
				out.WriteRune(r)
			}
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

// Hangul syllables in the Unicode Hangul Syllables block (AC00-D7A3) are
// algorithmically composed from an initial (choseong), medial (jungseong)
// and optional final (jongseong) jamo:
//
//	codepoint = 0xAC00 + (initial*21 + medial)*28 + final
//
// so they decompose back out with plain arithmetic, no lookup table of
// syllables needed, just tables for the 19 initials, 21 medials and 28
// finals (including "no final").
var hangulInitials = []string{
	"g", "kk", "n", "d", "tt", "r", "m", "b", "pp", "s", "ss", "", "j", "jj", "c", "k", "t", "p", "h",
}

var hangulMedials = []string{
	"a", "ae", "ya", "yae", "eo", "e", "yeo", "ye", "o", "wa", "wae", "oe",
	"yo", "u", "wo", "we", "wi", "yu", "eu", "ui", "i",
}

var hangulFinals = []string{
	"", "g", "kk", "gs", "n", "nj", "nh", "d", "l", "lg", "lm", "lb", "ls",
	"lt", "lp", "lh", "m", "b", "bs", "s", "ss", "ng", "j", "c", "k", "t", "p", "h",
}

func transliterateHangulSyllable(r rune) string {
	idx := int(r) - 0xAC00
	initial := idx / (21 * 28)
	medial := (idx / 28) % 21
	final := idx % 28
	return hangulInitials[initial] + hangulMedials[medial] + hangulFinals[final]
}

// cyrillicTable maps Russian-alphabet Cyrillic letters to Latin, using the
// same scheme most library systems and road signage use (a practical
// romanization, not a strict transliteration standard like GOST/ISO 9).
var cyrillicTable = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "yo",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "kh", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
	'А': "A", 'Б': "B", 'В': "V", 'Г': "G", 'Д': "D", 'Е': "E", 'Ё': "Yo",
	'Ж': "Zh", 'З': "Z", 'И': "I", 'Й': "Y", 'К': "K", 'Л': "L", 'М': "M",
	'Н': "N", 'О': "O", 'П': "P", 'Р': "R", 'С': "S", 'Т': "T", 'У': "U",
	'Ф': "F", 'Х': "Kh", 'Ц': "Ts", 'Ч': "Ch", 'Ш': "Sh", 'Щ': "Shch",
	'Ъ': "", 'Ы': "Y", 'Ь': "", 'Э': "E", 'Ю': "Yu", 'Я': "Ya",
}

// greekTable maps modern Greek letters to Latin following the common
// ELOT 743 / UN romanization used on Greek road signs and passports.
var greekTable = map[rune]string{
	'α': "a", 'β': "v", 'γ': "g", 'δ': "d", 'ε': "e", 'ζ': "z", 'η': "i",
	'θ': "th", 'ι': "i", 'κ': "k", 'λ': "l", 'μ': "m", 'ν': "n", 'ξ': "x",
	'ο': "o", 'π': "p", 'ρ': "r", 'σ': "s", 'ς': "s", 'τ': "t", 'υ': "y",
	'φ': "f", 'χ': "ch", 'ψ': "ps", 'ω': "o",
	'ά': "a", 'έ': "e", 'ή': "i", 'ί': "i", 'ό': "o", 'ύ': "y", 'ώ': "o",
	'ϊ': "i", 'ϋ': "y", 'ΐ': "i", 'ΰ': "y",
	'Α': "A", 'Β': "V", 'Γ': "G", 'Δ': "D", 'Ε': "E", 'Ζ': "Z", 'Η': "I",
	'Θ': "Th", 'Ι': "I", 'Κ': "K", 'Λ': "L", 'Μ': "M", 'Ν': "N", 'Ξ': "X",
	'Ο': "O", 'Π': "P", 'Ρ': "R", 'Σ': "S", 'Τ': "T", 'Υ': "Y",
	'Φ': "F", 'Χ': "Ch", 'Ψ': "Ps", 'Ω': "O",
	'Ά': "A", 'Έ': "E", 'Ή': "I", 'Ί': "I", 'Ό': "O", 'Ύ': "Y", 'Ώ': "O",
}
