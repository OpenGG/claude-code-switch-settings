package validator

import (
	"regexp"
	"strings"

	"github.com/OpenGG/claude-code-switch-settings/internal/ccs/domain"
)

var (
	reservedNamePattern = regexp.MustCompile(`^(?i)(con|prn|aux|nul|com[1-9]|lpt[1-9])$`)
	invalidCharsPattern = regexp.MustCompile(`[<>:"/\\|?*]`)
)

// Validator validates settings names for security and compatibility.
type Validator struct{}

// New creates a new Validator instance.
func New() *Validator {
	return &Validator{}
}

// ValidateName validates a settings name for security and compatibility.
//
// The function checks for:
//   - Empty names or whitespace-only names
//   - Dot navigation (. or ..)
//   - Null bytes (path traversal attack vector)
//   - Non-printable ASCII characters
//   - Invalid filesystem characters (<>:"/\|?*)
//   - Reserved Windows filenames (CON, PRN, AUX, NUL, COM1-9, LPT1-9)
//
// Returns (true, nil) if valid, or (false, error) with a descriptive error.
func (v *Validator) ValidateName(name string) (bool, error) {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) == 0 {
		return false, domain.ErrSettingsNameEmpty
	}
	if trimmed == "." || trimmed == ".." {
		return false, domain.ErrSettingsNameDot
	}

	// Explicit null byte check for defense-in-depth
	if strings.ContainsRune(trimmed, 0) {
		return false, domain.ErrSettingsNameNullByte
	}

	for _, r := range trimmed {
		if r < 0x20 || r > 0x7e {
			return false, domain.ErrSettingsNameNonPrintable
		}
		if r == 0x7f {
			return false, domain.ErrSettingsNameNonPrintable
		}
	}
	if invalidCharsPattern.MatchString(trimmed) {
		return false, domain.ErrSettingsNameInvalidChars
	}
	if reservedNamePattern.MatchString(trimmed) {
		return false, domain.ErrSettingsNameReserved
	}
	return true, nil
}

// NormalizeName trims whitespace and validates the name.
func (v *Validator) NormalizeName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if ok, err := v.ValidateName(trimmed); !ok {
		return "", err
	}
	return trimmed, nil
}
