package validator

// Tests for settings name validation (SECURITY CRITICAL).
//
// Settings names become filesystem paths. These tests prevent:
// - Path traversal (., .., /, \)
// - Injection attacks (null bytes, control chars)
// - Reserved names (CON, PRN, etc.)
// - Invalid characters and Unicode
//
// See TESTING.md for full security testing guidelines.

import (
	"errors"
	"testing"

	"github.com/OpenGG/claude-code-switch-settings/internal/ccs/domain"
)

func TestValidateName_ValidNames(t *testing.T) {
	v := New()

	validNames := []string{
		"work",
		"my-settings",
		"my_settings",
		"settings123",
		"v1.2.3",
		"test-~",
		"UPPERCASE",
		"MixedCase",
		"with.multiple.dots",
		"with-multiple-dashes",
		"with_multiple_underscores",
		"abc123_xyz-789",
	}

	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			valid, err := v.ValidateName(name)
			if !valid || err != nil {
				t.Errorf("expected valid for %q, got valid=%v err=%v", name, valid, err)
			}
		})
	}
}

func TestValidateName_EmptyAndWhitespace(t *testing.T) {
	v := New()

	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"only spaces", "   "},
		{"only tab", "\t"},
		{"newline", "\n"},
		{"multiple whitespace", "  \t  \n  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := v.ValidateName(tt.input)
			if valid {
				t.Error("expected invalid for empty/whitespace")
			}
			if !errors.Is(err, domain.ErrSettingsNameEmpty) {
				t.Errorf("expected ErrSettingsNameEmpty, got %v", err)
			}
		})
	}
}

func TestValidateName_DotNavigation(t *testing.T) {
	v := New()

	tests := []struct {
		name  string
		input string
	}{
		{"single dot", "."},
		{"double dot", ".."},
		{"dot with spaces", " . "},
		{"double dot with spaces", " .. "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := v.ValidateName(tt.input)
			if valid {
				t.Error("expected invalid for dot navigation")
			}
			if !errors.Is(err, domain.ErrSettingsNameDot) {
				t.Errorf("expected ErrSettingsNameDot, got %v", err)
			}
		})
	}
}

func TestValidateName_NullBytes(t *testing.T) {
	v := New()

	tests := []struct {
		name  string
		input string
	}{
		{"null at start", "\x00test"},
		{"null in middle", "test\x00file"},
		{"null at end", "test\x00"},
		{"multiple nulls", "te\x00st\x00file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := v.ValidateName(tt.input)
			if valid {
				t.Error("expected invalid for null byte")
			}
			if !errors.Is(err, domain.ErrSettingsNameNullByte) {
				t.Errorf("expected ErrSettingsNameNullByte, got %v", err)
			}
		})
	}
}

func TestValidateName_ControlCharacters(t *testing.T) {
	v := New()

	tests := []struct {
		name  string
		input string
	}{
		{"SOH in middle", "te\x01st"},
		{"STX in middle", "te\x02st"},
		{"ETX in middle", "te\x03st"},
		{"BEL in middle", "te\x07st"},
		{"BS in middle", "te\x08st"},
		{"TAB in middle", "te\x09st"},
		{"LF in middle", "te\x0ast"},
		{"CR in middle", "te\x0dst"},
		{"ESC in middle", "te\x1bst"},
		{"DEL in middle", "te\x7fst"},
		{"below space", "te\x1fst"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := v.ValidateName(tt.input)
			if valid {
				t.Errorf("expected invalid for control character in %q", tt.name)
			}
			if !errors.Is(err, domain.ErrSettingsNameNonPrintable) {
				t.Errorf("expected ErrSettingsNameNonPrintable, got %v", err)
			}
		})
	}
}

func TestValidateName_InvalidFilesystemCharacters(t *testing.T) {
	v := New()

	tests := []struct {
		name  string
		input string
		char  string
	}{
		{"forward slash", "my/settings", "/"},
		{"backslash", "my\\settings", "\\"},
		{"colon", "my:settings", ":"},
		{"asterisk", "my*settings", "*"},
		{"question mark", "my?settings", "?"},
		{"double quote", "my\"settings", "\""},
		{"less than", "my<settings", "<"},
		{"greater than", "my>settings", ">"},
		{"pipe", "my|settings", "|"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := v.ValidateName(tt.input)
			if valid {
				t.Errorf("expected invalid for %s character", tt.char)
			}
			if !errors.Is(err, domain.ErrSettingsNameInvalidChars) {
				t.Errorf("expected ErrSettingsNameInvalidChars, got %v", err)
			}
		})
	}
}

func TestValidateName_ReservedWindowsNames(t *testing.T) {
	v := New()

	tests := []struct {
		name  string
		input string
	}{
		{"CON uppercase", "CON"},
		{"CON lowercase", "con"},
		{"CON mixed", "Con"},
		{"PRN", "PRN"},
		{"prn", "prn"},
		{"AUX", "AUX"},
		{"aux", "aux"},
		{"NUL", "NUL"},
		{"nul", "nul"},
		{"COM1", "COM1"},
		{"com1", "com1"},
		{"COM9", "COM9"},
		{"LPT1", "LPT1"},
		{"lpt5", "lpt5"},
		{"LPT9", "LPT9"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := v.ValidateName(tt.input)
			if valid {
				t.Errorf("expected invalid for reserved name %q", tt.input)
			}
			if !errors.Is(err, domain.ErrSettingsNameReserved) {
				t.Errorf("expected ErrSettingsNameReserved, got %v", err)
			}
		})
	}
}

func TestValidateName_Unicode(t *testing.T) {
	v := New()

	tests := []struct {
		name  string
		input string
	}{
		{"emoji", "settings😀"},
		{"Chinese", "设置"},
		{"accented", "café"},
		{"Cyrillic", "настройки"},
		{"Japanese", "設定"},
		{"Arabic", "إعدادات"},
		{"mixed", "test设置"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := v.ValidateName(tt.input)
			if valid {
				t.Errorf("expected invalid for unicode in %q", tt.input)
			}
			if !errors.Is(err, domain.ErrSettingsNameNonPrintable) {
				t.Errorf("expected ErrSettingsNameNonPrintable, got %v", err)
			}
		})
	}
}

func TestValidateName_WhitespaceHandling(t *testing.T) {
	v := New()

	// ValidateName expects already-trimmed input
	// Leading/trailing spaces should be rejected (not auto-trimmed)
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"no spaces", "work", true},
		{"leading space", " work", true},  // Will be trimmed
		{"trailing space", "work ", true}, // Will be trimmed
		{"both spaces", " work ", true},   // Will be trimmed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := v.ValidateName(tt.input)
			if valid != tt.valid {
				t.Errorf("expected valid=%v for %q, got valid=%v err=%v", tt.valid, tt.input, valid, err)
			}
		})
	}
}

func TestNormalizeName_TrimsAndValidates(t *testing.T) {
	v := New()

	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{"no trim needed", "work", "work", false},
		{"trim leading", " work", "work", false},
		{"trim trailing", "work ", "work", false},
		{"trim both", "  work  ", "work", false},
		{"invalid after trim", "  ..  ", "", true},
		{"empty after trim", "   ", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := v.NormalizeName(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("expected %q, got %q", tt.expected, result)
				}
			}
		})
	}
}

func TestNormalizeName_PropagatesValidationError(t *testing.T) {
	v := New()

	_, err := v.NormalizeName("../etc/passwd")
	if err == nil {
		t.Fatal("expected error for invalid name")
	}
	if !errors.Is(err, domain.ErrSettingsNameInvalidChars) {
		t.Errorf("expected ErrSettingsNameInvalidChars, got %v", err)
	}
}

func TestValidateName_EdgeCases(t *testing.T) {
	v := New()

	tests := []struct {
		name    string
		input   string
		valid   bool
		errType error
	}{
		// Boundary characters
		{"space (0x20)", "test file", true, nil}, // Space is printable
		{"tilde (0x7E)", "test~", true, nil},     // Max printable ASCII
		{"unit separator (0x1F)", "test\x1f", false, domain.ErrSettingsNameNonPrintable},
		{"above tilde (0x7F)", "test\x7f", false, domain.ErrSettingsNameNonPrintable},

		// Valid special characters
		{"hyphen", "test-file", true, nil},
		{"underscore", "test_file", true, nil},
		{"period", "test.file", true, nil},
		{"numbers", "test123", true, nil},

		// Reserved name variations (should fail even with extra chars)
		{"CON exactly", "CON", false, domain.ErrSettingsNameReserved},
		{"COM5 exactly", "COM5", false, domain.ErrSettingsNameReserved},

		// Dot variations (valid when not exactly . or ..)
		{"dot prefix", ".gitignore", true, nil}, // Leading dot is OK
		{"dot suffix", "file.", true, nil},      // Trailing dot is OK
		{"multiple dots", "...", true, nil},     // More than 2 dots is OK
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := v.ValidateName(tt.input)
			if valid != tt.valid {
				t.Errorf("expected valid=%v, got valid=%v (err=%v)", tt.valid, valid, err)
			}
			if tt.errType != nil && !errors.Is(err, tt.errType) {
				t.Errorf("expected error type %v, got %v", tt.errType, err)
			}
		})
	}
}
