package ccs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

var (
	fsProvider   = afero.NewOsFs()
	exitFunc     = os.Exit
	rootFactory  = func() *cobra.Command { return newRootCmd() }
	selectRunner = func(sel *promptui.Select) (int, string, error) {
		return sel.Run()
	}
	promptRunner = func(pr *promptui.Prompt) (string, error) {
		return pr.Run()
	}
)

// Execute runs the CLI application.
func Execute() {
	if err := rootFactory().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitFunc(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ccs",
		Short: "Claude Code Switcher",
		Long:  "ccs manages and switches between multiple Claude Code settings.",
	}

	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newUseCmd())
	cmd.AddCommand(newSaveCmd())
	cmd.AddCommand(newPruneBackupsCmd())

	return cmd
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := createService()
			if err != nil {
				return err
			}
			lines, err := service.ListSettings()
			if err != nil {
				return err
			}
			if len(lines) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No settings stored yet.")
				return nil
			}
			for _, line := range lines {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
}

func newUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use [name]",
		Short: "Load and activate a stored settings profile",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := createService()
			if err != nil {
				return err
			}

			var name string
			if len(args) == 1 {
				name = args[0]
			} else {
				name, err = promptForSettingsName(service, "Select settings to activate")
				if err != nil {
					return err
				}
			}

			if name == "" {
				return errors.New("No settings selected")
			}

			if err := service.UseSettings(name); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Successfully switched to settings: %s\n", name)
			return nil
		},
	}
}

func newSaveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "save",
		Short: "Save and activate the current settings.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := createService()
			if err != nil {
				return err
			}

			paths := service.Paths()
			if _, err := fsProvider.Stat(paths.ActiveSettingsPath); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return errors.New("Error: settings.json not found. Nothing to save.")
				}
				return err
			}

			names, err := service.ListStoredNames()
			if err != nil {
				return err
			}

			const newOption = "[New Settings]"
			options := append(names, newOption)

			active, err := service.GetActiveSettingsName()
			if err != nil {
				return err
			}

			cursor := 0
			for i, n := range options {
				if n == active {
					cursor = i
					break
				}
			}

			var selected string
			if isAutomation() {
				if value, ok := automationValue("CCS_SELECT_SETTING"); ok {
					for _, option := range options {
						if option == value {
							selected = value
							break
						}
					}
					if selected == "" {
						return fmt.Errorf("automation selection '%s' not available", value)
					}
				} else if active != "" {
					selected = active
				} else {
					selected = options[0]
				}
			} else {
				prompt := promptui.Select{
					Label:     "Select settings to save",
					Items:     options,
					CursorPos: cursor,
				}

				_, runSelected, err := selectRunner(&prompt)
				if err != nil {
					return err
				}
				selected = runSelected
			}

			var targetName string
			if selected == newOption {
				targetName, err = promptForNewSettingsName(service)
				if err != nil {
					return err
				}
			} else {
				targetName = selected
				if !isAutomation() {
					confirmPrompt := promptui.Prompt{
						Label:     fmt.Sprintf("Overwrite %s?", targetName),
						IsConfirm: true,
					}
					result, err := promptRunner(&confirmPrompt)
					if err != nil {
						if err == promptui.ErrAbort {
							return errors.New("Operation cancelled")
						}
						return err
					}
					if strings.ToLower(result) != "y" && result != "" {
						return errors.New("Operation cancelled")
					}
				}
			}

			if err := service.SaveSettings(targetName); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Successfully saved and activated settings: %s\n", targetName)
			return nil
		},
	}
}

func newPruneBackupsCmd() *cobra.Command {
	var olderThanStr string
	var force bool

	cmd := &cobra.Command{
		Use:   "prune-backups",
		Short: "Prune old backup files",
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := createService()
			if err != nil {
				return err
			}

			duration, err := resolveOlderThan(olderThanStr)
			if err != nil {
				return err
			}

			if duration == 0 {
				selection, err := promptForPruneDuration()
				if err != nil {
					return err
				}
				duration = selection
			}

			if !force && !isAutomation() {
				confirmPrompt := promptui.Prompt{
					Label:     fmt.Sprintf("Remove backups older than %s?", humanizeDuration(duration)),
					IsConfirm: true,
				}
				result, err := promptRunner(&confirmPrompt)
				if err != nil {
					if err == promptui.ErrAbort {
						return errors.New("Operation cancelled")
					}
					return err
				}
				if strings.ToLower(result) != "y" && result != "" {
					return errors.New("Operation cancelled")
				}
			}

			removed, err := service.PruneBackups(duration)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed %d backup(s).\n", removed)
			return nil
		},
	}

	cmd.Flags().StringVar(&olderThanStr, "older-than", "", "Duration threshold for pruning (e.g. 30d, 48h)")
	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompts")
	return cmd
}

func resolveOlderThan(input string) (time.Duration, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return 0, nil
	}

	suffix := trimmed[len(trimmed)-1]
	value := trimmed[:len(trimmed)-1]
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("Invalid duration: %s", input)
	}

	switch suffix {
	case 'd', 'D':
		return time.Duration(n) * 24 * time.Hour, nil
	case 'h', 'H':
		return time.Duration(n) * time.Hour, nil
	case 'm', 'M':
		return time.Duration(n) * time.Minute, nil
	default:
		return 0, fmt.Errorf("Unsupported duration suffix: %c", suffix)
	}
}

func promptForPruneDuration() (time.Duration, error) {
	type option struct {
		Label    string
		Duration time.Duration
	}

	options := []option{
		{Label: "30 days", Duration: 30 * 24 * time.Hour},
		{Label: "60 days", Duration: 60 * 24 * time.Hour},
		{Label: "90 days", Duration: 90 * 24 * time.Hour},
	}

	if isAutomation() {
		if value, ok := automationValue("CCS_PRUNE_DURATION"); ok {
			duration, err := resolveOlderThan(value)
			if err != nil {
				return 0, err
			}
			return duration, nil
		}
		return 30 * 24 * time.Hour, nil
	}

	prompt := promptui.Select{
		Label: "Select backup retention threshold",
		Items: options,
		Templates: &promptui.SelectTemplates{
			Label:    "{{ . }}",
			Active:   "▸ {{ .Label }}",
			Inactive: "  {{ .Label }}",
			Selected: "Retention: {{ .Label }}",
		},
	}

	index, _, err := selectRunner(&prompt)
	if err != nil {
		return 0, err
	}

	return options[index].Duration, nil
}

func humanizeDuration(d time.Duration) string {
	hours := int(d.Hours())
	if hours%24 == 0 {
		days := hours / 24
		return fmt.Sprintf("%dd", days)
	}
	if hours%1 == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return d.String()
}

func promptForSettingsName(service *Service, label string) (string, error) {
	names, err := service.ListStoredNames()
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", errors.New("No stored settings available")
	}

	if isAutomation() {
		if value, ok := automationValue("CCS_SELECT_SETTING"); ok {
			for _, name := range names {
				if name == value {
					return value, nil
				}
			}
			return "", fmt.Errorf("automation selection '%s' not found", value)
		}
		return names[0], nil
	}

	prompt := promptui.Select{
		Label: label,
		Items: names,
	}

	_, selected, err := selectRunner(&prompt)
	if err != nil {
		return "", err
	}
	return selected, nil
}

func promptForNewSettingsName(service *Service) (string, error) {
	existing, err := service.ListStoredNames()
	if err != nil {
		return "", err
	}
	existingSet := make(map[string]struct{}, len(existing))
	for _, name := range existing {
		existingSet[name] = struct{}{}
	}

	if isAutomation() {
		if value, ok := automationValue("CCS_NEW_SETTINGS_NAME"); ok {
			valid, err := ValidateSettingsName(value)
			if !valid {
				return "", err
			}
			if _, exists := existingSet[value]; exists {
				return "", fmt.Errorf("Settings '%s' already exists", value)
			}
			return value, nil
		}
		return "", errors.New("automation requires CCS_NEW_SETTINGS_NAME")
	}

	for {
		prompt := promptui.Prompt{Label: "Enter new settings name"}
		input, err := promptRunner(&prompt)
		if err != nil {
			if err == promptui.ErrAbort {
				return "", errors.New("Operation cancelled")
			}
			return "", err
		}

		valid, vErr := ValidateSettingsName(input)
		if !valid {
			fmt.Println("Error:", vErr.Error())
			continue
		}

		if _, exists := existingSet[input]; exists {
			fmt.Printf("Error: Settings '%s' already exists.\n", input)
			continue
		}

		return input, nil
	}
}

func createService() (*Service, error) {
	base, err := resolveBaseDir()
	if err != nil {
		return nil, err
	}
	return NewService(fsProvider, base)
}

func resolveBaseDir() (string, error) {
	if override := os.Getenv("CCS_BASE_DIR"); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, claudeDirName), nil
}

func isAutomation() bool {
	return os.Getenv("CCS_NON_INTERACTIVE") == "1"
}

func automationValue(key string) (string, bool) {
	value := os.Getenv(key)
	if value == "" {
		return "", false
	}
	return value, true
}
