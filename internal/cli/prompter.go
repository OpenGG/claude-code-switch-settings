package cli

type Prompter interface {
	Select(label string, items []string, defaultValue string) (int, string, error)
	Prompt(label string) (string, error)
	Confirm(label string, defaultYes bool) (bool, error)
}
