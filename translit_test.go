package mewsync

import "testing"

func TestTransliterateCyrillic(t *testing.T) {
	cases := map[string]string{
		"привет": "privet",
		"Москва": "Moskva",
		"Россия": "Rossiya",
		"щи":     "shchi",
	}
	for in, want := range cases {
		if got := Transliterate(in); got != want {
			t.Errorf("Transliterate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTransliterateGreek(t *testing.T) {
	cases := map[string]string{
		"γεια":  "geia",
		"Αθήνα": "Athina",
		"ψυχή":  "psychi",
	}
	for in, want := range cases {
		if got := Transliterate(in); got != want {
			t.Errorf("Transliterate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTransliterateHangul(t *testing.T) {
	cases := map[string]string{
		"한글": "hangeul",
		"안녕": "annyeong",
		"사랑": "sarang",
	}
	for in, want := range cases {
		if got := Transliterate(in); got != want {
			t.Errorf("Transliterate(%q) = %q, want %q", in, got, want)
		}
	}
}

// Han characters (Chinese hanzi / Japanese kanji) have no small deterministic
// romanization algorithm, so they must pass through unchanged rather than
// being silently mangled.
func TestTransliterateHanPassthrough(t *testing.T) {
	cases := []string{"你好", "日本語", "漢字"}
	for _, in := range cases {
		if got := Transliterate(in); got != in {
			t.Errorf("Transliterate(%q) = %q, want unchanged", in, got)
		}
	}
}

func TestTransliterateMixed(t *testing.T) {
	in := "Hello Привет 안녕 你好"
	want := "Hello Privet annyeong 你好"
	if got := Transliterate(in); got != want {
		t.Errorf("Transliterate(%q) = %q, want %q", in, got, want)
	}
}
