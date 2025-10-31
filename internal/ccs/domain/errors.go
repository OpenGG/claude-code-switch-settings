package domain

import "errors"

// Exported error variables allow callers to use errors.Is() for error checking.
var (
	ErrSettingsNameEmpty        = errors.New("settings name cannot be empty")
	ErrSettingsNameDot          = errors.New("settings name cannot be '.' or '..'")
	ErrSettingsNameNonPrintable = errors.New("settings name contains non-printable characters")
	ErrSettingsNameInvalidChars = errors.New("settings name contains invalid characters (<>:\"/|?*)")
	ErrSettingsNameReserved     = errors.New("settings name is a reserved system filename")
	ErrSettingsNameNullByte     = errors.New("settings name contains null byte")
)
