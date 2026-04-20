package horisen

// DCS is the data coding scheme Horisen accepts in the `dcs` field.
type DCS string

const (
	DCSGSM DCS = "GSM" // 7-bit GSM default alphabet (+ extension table)
	DCSUCS DCS = "UCS" // UCS-2 (UTF-16) for anything outside GSM-7
)

// gsm7Basic is the 128-character GSM 03.38 default alphabet.
// Each rune in this set counts as 1 septet.
var gsm7Basic = map[rune]struct{}{
	'@': {}, '£': {}, '$': {}, '¥': {}, 'è': {}, 'é': {}, 'ù': {}, 'ì': {},
	'ò': {}, 'Ç': {}, '\n': {}, 'Ø': {}, 'ø': {}, '\r': {}, 'Å': {}, 'å': {},
	'Δ': {}, '_': {}, 'Φ': {}, 'Γ': {}, 'Λ': {}, 'Ω': {}, 'Π': {}, 'Ψ': {},
	'Σ': {}, 'Θ': {}, 'Ξ': {}, 'Æ': {}, 'æ': {}, 'ß': {}, 'É': {},
	' ': {}, '!': {}, '"': {}, '#': {}, '¤': {}, '%': {}, '&': {}, '\'': {},
	'(': {}, ')': {}, '*': {}, '+': {}, ',': {}, '-': {}, '.': {}, '/': {},
	'0': {}, '1': {}, '2': {}, '3': {}, '4': {}, '5': {}, '6': {}, '7': {},
	'8': {}, '9': {}, ':': {}, ';': {}, '<': {}, '=': {}, '>': {}, '?': {},
	'¡': {}, 'A': {}, 'B': {}, 'C': {}, 'D': {}, 'E': {}, 'F': {}, 'G': {},
	'H': {}, 'I': {}, 'J': {}, 'K': {}, 'L': {}, 'M': {}, 'N': {}, 'O': {},
	'P': {}, 'Q': {}, 'R': {}, 'S': {}, 'T': {}, 'U': {}, 'V': {}, 'W': {},
	'X': {}, 'Y': {}, 'Z': {}, 'Ä': {}, 'Ö': {}, 'Ñ': {}, 'Ü': {}, '§': {},
	'¿': {}, 'a': {}, 'b': {}, 'c': {}, 'd': {}, 'e': {}, 'f': {}, 'g': {},
	'h': {}, 'i': {}, 'j': {}, 'k': {}, 'l': {}, 'm': {}, 'n': {}, 'o': {},
	'p': {}, 'q': {}, 'r': {}, 's': {}, 't': {}, 'u': {}, 'v': {}, 'w': {},
	'x': {}, 'y': {}, 'z': {}, 'ä': {}, 'ö': {}, 'ñ': {}, 'ü': {}, 'à': {},
}

// gsm7Ext is the GSM extension table. Each rune here counts as 2 septets.
var gsm7Ext = map[rune]struct{}{
	'\f': {}, '^': {}, '{': {}, '}': {}, '\\': {}, '[': {}, '~': {}, ']': {},
	'|': {}, '€': {},
}

// DetectDCS returns DCSGSM if every rune fits in GSM-7 (basic or extension),
// otherwise DCSUCS.
func DetectDCS(text string) DCS {
	for _, r := range text {
		if _, ok := gsm7Basic[r]; ok {
			continue
		}
		if _, ok := gsm7Ext[r]; ok {
			continue
		}
		return DCSUCS
	}
	return DCSGSM
}

// NumParts returns how many concatenated SMS parts the text will take.
// GSM-7: 160 septets single / 153 per part when split.
// UCS-2: 70 code units single / 67 per part when split.
func NumParts(text string, dcs DCS) int {
	if text == "" {
		return 1
	}
	switch dcs {
	case DCSGSM:
		septets := 0
		for _, r := range text {
			if _, ok := gsm7Ext[r]; ok {
				septets += 2
			} else {
				septets++
			}
		}
		if septets <= 160 {
			return 1
		}
		return (septets + 152) / 153
	case DCSUCS:
		// UCS-2 counts UTF-16 code units. Runes above 0xFFFF take 2 units (surrogate pair).
		units := 0
		for _, r := range text {
			units++
			if r > 0xFFFF {
				units++
			}
		}
		if units <= 70 {
			return 1
		}
		return (units + 66) / 67
	}
	return 1
}
