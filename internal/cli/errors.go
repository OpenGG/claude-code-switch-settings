package cli

import "errors"

// ErrPromptCancelled indicates that the user aborted an interactive prompt.
var ErrPromptCancelled = errors.New("prompt cancelled")
