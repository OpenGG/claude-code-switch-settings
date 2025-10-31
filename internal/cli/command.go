package cli

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/OpenGG/claude-code-switch-settings/internal/ccs"
)

// NewRootCommand constructs the root Cobra command for ccs.
func NewRootCommand(mgr *ccs.Manager, prompter Prompter, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ccs",
		Short: "Claude Code Switcher",
		Long:  "ccs helps manage multiple Claude Code settings safely.",
	}

	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	cmd.AddCommand(newListCommand(mgr, stdout))
	cmd.AddCommand(newUseCommand(mgr, prompter, stdout))
	cmd.AddCommand(newSaveCommand(mgr, prompter))
	cmd.AddCommand(newPruneCommand(mgr, prompter, stdout))

	return cmd
}

func newListCommand(mgr *ccs.Manager, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := mgr.ListSettings()
			if err != nil {
				return err
			}
			for _, entry := range entries {
				qualifier := ""
				if len(entry.Qualifiers) > 0 {
					qualifier = " (" + strings.Join(entry.Qualifiers, ", ") + ")"
				}
				if entry.Plain {
					fmt.Fprintf(stdout, "%s %s%s\n", entry.Prefix, entry.Name, qualifier)
				} else {
					fmt.Fprintf(stdout, "%s [%s]%s\n", entry.Prefix, entry.Name, qualifier)
				}
			}
			if len(entries) == 0 {
				fmt.Fprintln(stdout, "No saved settings found.")
			}
			return nil
		},
	}
}

func newUseCommand(mgr *ccs.Manager, prompter Prompter, stdout io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use [name]",
		Short: "Load and activate a stored settings profile",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
				// Early validation of command-line argument
				if valid, err := mgr.ValidateSettingsName(name); !valid {
					return fmt.Errorf("invalid settings name: %w", err)
				}
			} else {
				names, err := mgr.StoredSettings()
				if err != nil {
					return err
				}
				if len(names) == 0 {
					return fmt.Errorf("use command: no stored settings available in %s", mgr.SettingsStoreDir())
				}
				names = reorderWithDefault(names, mgr.GetActiveSettingsName())
				_, selected, err := prompter.Select("Select settings to activate", names, mgr.GetActiveSettingsName())
				if err != nil {
					return err
				}
				name = selected
			}
			if err := mgr.Use(name); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "Successfully switched to settings: %s\n", name)
			return nil
		},
	}
	return cmd
}

const newSettingsLabel = "[New Settings]"

func newSaveCommand(mgr *ccs.Manager, prompter Prompter) *cobra.Command {
	return &cobra.Command{
		Use:   "save",
		Short: "Save current settings and activate them",
		RunE: func(cmd *cobra.Command, args []string) error {
			if exists, err := afero.Exists(mgr.FileSystem(), mgr.ActiveSettingsPath()); err != nil {
				return fmt.Errorf("failed to inspect settings.json: %w", err)
			} else if !exists {
				return errors.New("settings.json not found. Nothing to save.")
			}

			names, err := mgr.StoredSettings()
			if err != nil {
				return err
			}
			defaultValue := mgr.GetActiveSettingsName()
			if defaultValue == "" {
				defaultValue = newSettingsLabel
			}
			names = reorderWithDefault(names, defaultValue)
			items := append([]string{newSettingsLabel}, names...)
			_, selection, err := prompter.Select("Select destination to save current settings", items, defaultValue)
			if err != nil {
				return err
			}

			target := selection
			if selection == newSettingsLabel {
				for {
					name, err := prompter.Prompt("Enter a name for the new settings")
					if err != nil {
						return err
					}
					name = strings.TrimSpace(name)
					valid, vErr := mgr.ValidateSettingsName(name)
					if !valid {
						fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s\n", vErr.Error())
						continue
					}
					path, err := mgr.StoredSettingsPath(name)
					if err != nil {
						return err
					}
					if exists, err := afero.Exists(mgr.FileSystem(), path); err != nil {
						return err
					} else if exists {
						fmt.Fprintf(cmd.ErrOrStderr(), "Error: Settings '%s' already exists.\n", name)
						continue
					}
					target = name
					break
				}
			} else {
				confirm, err := prompter.Confirm(fmt.Sprintf("Overwrite %s? (y/N)", selection), false)
				if err != nil {
					return err
				}
				if !confirm {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted saving settings.")
					return nil
				}
			}

			if err := mgr.Save(target); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Successfully saved and activated settings: %s\n", target)
			return nil
		},
	}
}

func newPruneCommand(mgr *ccs.Manager, prompter Prompter, stdout io.Writer) *cobra.Command {
	var olderThanStr string
	var force bool

	cmd := &cobra.Command{
		Use:   "prune-backups",
		Short: "Remove outdated backup files",
		RunE: func(cmd *cobra.Command, args []string) error {
			var duration time.Duration
			var err error

			if olderThanStr != "" {
				duration, err = parseHumanDuration(olderThanStr)
				if err != nil {
					return err
				}
			} else {
				options := []string{"30d", "90d", "180d", "Cancel"}
				_, choice, err := prompter.Select("Prune backups older than", options, "30d")
				if err != nil {
					return err
				}
				if choice == "Cancel" {
					fmt.Fprintln(stdout, "Prune cancelled.")
					return nil
				}
				duration, err = parseHumanDuration(choice)
				if err != nil {
					return err
				}
			}

			if !force {
				confirm, err := prompter.Confirm(fmt.Sprintf("Delete backups older than %s? (y/N)", duration), false)
				if err != nil {
					return err
				}
				if !confirm {
					fmt.Fprintln(stdout, "Prune cancelled.")
					return nil
				}
			}

			count, err := mgr.PruneBackups(duration)
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "Deleted %d backup(s).\n", count)
			return nil
		},
	}

	cmd.Flags().StringVar(&olderThanStr, "older-than", "", "Delete backups older than the specified duration (e.g. 30d)")
	cmd.Flags().BoolVar(&force, "force", false, "Do not prompt for confirmation")

	return cmd
}

func parseHumanDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return 0, errors.New("duration cannot be empty")
	}
	if strings.HasSuffix(value, "d") {
		days := strings.TrimSuffix(value, "d")
		v, err := parseDays(days)
		if err != nil {
			return 0, fmt.Errorf("invalid day duration: %w", err)
		}
		return v, nil
	}
	if strings.HasSuffix(value, "h") || strings.HasSuffix(value, "m") || strings.HasSuffix(value, "s") {
		dur, err := time.ParseDuration(value)
		if err != nil {
			return 0, err
		}
		if dur < 0 {
			return 0, fmt.Errorf("duration cannot be negative")
		}
		return dur, nil
	}
	return 0, fmt.Errorf("unsupported duration format: %s", value)
}

func parseDays(value string) (time.Duration, error) {
	d, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid day duration: %w", err)
	}
	if d < 0 {
		return 0, fmt.Errorf("invalid day duration: %d", d)
	}
	return time.Duration(d) * 24 * time.Hour, nil
}

// reorderWithDefault moves the default value to the front of the list.
// If defaultValue is empty or not found, or already first, returns items unchanged.
func reorderWithDefault(items []string, defaultValue string) []string {
	if defaultValue == "" {
		return items
	}

	// Find the index of the default value
	idx := -1
	for i, item := range items {
		if item == defaultValue {
			idx = i
			break
		}
	}

	// If not found or already at position 0, return unchanged
	if idx <= 0 {
		return items
	}

	// Build reordered list: [defaultValue, items before idx, items after idx]
	reordered := make([]string, 0, len(items))
	reordered = append(reordered, defaultValue)
	reordered = append(reordered, items[:idx]...)
	reordered = append(reordered, items[idx+1:]...)

	return reordered
}
