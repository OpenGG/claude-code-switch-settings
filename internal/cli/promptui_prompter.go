package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/manifoldco/promptui"
)

const (
	// defaultMenuSize is the number of items visible in selection menus
	defaultMenuSize = 10
)

type PromptUI struct {
	stdin  io.ReadCloser
	stdout io.WriteCloser
}

func NewPromptUI() *PromptUI {
	return &PromptUI{stdin: os.Stdin, stdout: os.Stdout}
}

func NewPromptUIWithIO(stdin io.Reader, stdout io.Writer) *PromptUI {
	pu := &PromptUI{stdin: os.Stdin, stdout: os.Stdout}
	if stdin != nil {
		pu.stdin = toReadCloser(stdin)
	}
	if stdout != nil {
		pu.stdout = toWriteCloser(stdout)
	}
	return pu
}

func (p *PromptUI) Select(label string, items []string, defaultValue string) (int, string, error) {
	cursor := 0
	if defaultValue != "" {
		for i, item := range items {
			if item == defaultValue {
				cursor = i
				break
			}
		}
	}

	selectPrompt := promptui.Select{
		Label:     label,
		Items:     items,
		Size:      defaultMenuSize,
		HideHelp:  true,
		CursorPos: cursor,
		Stdin:     p.stdin,
		Stdout:    p.stdout,
	}

	idx, value, err := selectPrompt.Run()
	if err != nil {
		return idx, value, fmt.Errorf("%w: %v", ErrPromptCancelled, err)
	}
	return idx, value, nil
}

func (p *PromptUI) Prompt(label string) (string, error) {
	prompt := promptui.Prompt{
		Label:  label,
		Stdin:  p.stdin,
		Stdout: p.stdout,
	}
	value, err := prompt.Run()
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPromptCancelled, err)
	}
	return value, nil
}

func (p *PromptUI) Confirm(label string, defaultYes bool) (bool, error) {
	def := "N"
	if defaultYes {
		def = "Y"
	}
	prompt := promptui.Prompt{
		Label:     label,
		IsConfirm: true,
		Default:   def,
		Stdin:     p.stdin,
		Stdout:    p.stdout,
	}
	result, err := prompt.Run()
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrPromptCancelled, err)
	}
	return strings.EqualFold(result, "y") || (result == "" && defaultYes), nil
}

func toReadCloser(r io.Reader) io.ReadCloser {
	if rc, ok := r.(io.ReadCloser); ok {
		return rc
	}
	return io.NopCloser(r)
}

func toWriteCloser(w io.Writer) io.WriteCloser {
	if wc, ok := w.(io.WriteCloser); ok {
		return wc
	}
	return nopWriteCloser{Writer: w}
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error {
	return nil
}
