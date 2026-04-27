package admin

import (
	"errors"
	"unicode"
)

// ValidatePassword enforces the admin password policy. Requirements:
//   - >= 12 characters
//   - at least one letter and one digit
//
// We deliberately avoid forcing symbols / casing — research (and NIST
// SP 800-63B) shows length is the dominant factor and complexity rules
// push users toward predictable patterns. We do block known-trivial
// passwords listed in trivialPasswords below.
func ValidatePassword(p string) error {
	if len(p) < 12 {
		return errors.New("password must be at least 12 characters long")
	}
	if len(p) > 200 {
		return errors.New("password too long (>200 chars)")
	}
	var hasLetter, hasDigit bool
	for _, r := range p {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return errors.New("password must contain at least one letter and one digit")
	}
	for _, bad := range trivialPasswords {
		if p == bad {
			return errors.New("password is on the common-password blocklist; pick another")
		}
	}
	return nil
}

// trivialPasswords — minimal embedded blocklist. The point isn't to be
// exhaustive (rockyou.txt has 14M entries) but to catch lazy choices like
// "Password1234". A full check would call out to a haveibeenpwned k-anon
// endpoint; out of scope for now.
var trivialPasswords = []string{
	"password1234", "Password1234",
	"administrator1", "Administrator1",
	"qwerty123456", "12345678abcd", "abcd12345678",
	"changeme1234", "Changeme1234",
	"sms-server-1", "SmsServer1234",
}
