package horisen

import (
	"strings"
	"testing"
)

func TestDetectDCS(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want DCS
	}{
		{"plain ascii", "Hello world", DCSGSM},
		{"digits+punct", "1234567890 !?@#", DCSGSM},
		{"spanish with accents in gsm basic", "¿Cómo?", DCSUCS}, // ó is not in GSM basic, only ò/é/è/ù/ì are
		{"gsm basic accents", "àäöñü", DCSGSM},
		{"gsm extension", "12€ [test]", DCSGSM},
		{"emoji forces ucs", "Hi 👋", DCSUCS},
		{"cyrillic forces ucs", "Привет", DCSUCS},
		{"chinese forces ucs", "你好", DCSUCS},
		{"empty", "", DCSGSM},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DetectDCS(c.in); got != c.want {
				t.Errorf("DetectDCS(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestNumParts_GSM(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"single", "hello", 1},
		{"exactly 160", strings.Repeat("a", 160), 1},
		{"161 chars splits", strings.Repeat("a", 161), 2},
		{"exactly 306 = 2 parts", strings.Repeat("a", 306), 2},
		{"307 chars = 3 parts", strings.Repeat("a", 307), 3},
		{"extension char counts double", strings.Repeat("€", 80), 1},    // 160 septets exactly
		{"extension char 81 splits", strings.Repeat("€", 81), 2},        // 162 septets
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NumParts(c.in, DCSGSM); got != c.want {
				t.Errorf("NumParts(%q len=%d, GSM) = %d, want %d", c.name, len(c.in), got, c.want)
			}
		})
	}
}

func TestNumParts_UCS(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 1},
		{"under 70 bmp", "Привет, mundo!", 1},
		{"exactly 70 bmp", strings.Repeat("ñ", 70), 1},
		{"71 bmp splits", strings.Repeat("ñ", 71), 2},
		{"emoji uses surrogate pair", strings.Repeat("👋", 35), 1}, // 35*2 = 70 code units
		{"emoji 36 splits", strings.Repeat("👋", 36), 2},           // 72 code units
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NumParts(c.in, DCSUCS); got != c.want {
				t.Errorf("NumParts(%q, UCS) = %d, want %d", c.name, got, c.want)
			}
		})
	}
}
