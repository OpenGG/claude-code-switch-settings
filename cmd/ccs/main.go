package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"

	"github.com/example/claude-code-switch-settings/internal/ccs"
)

var (
	selectFunc = func(label string, items []string) (int, string, error) {
		prompt := promptui.Select{
			Label: label,
			Items: items,
		}
		return prompt.Run()
	}
	promptFunc = func(label string, isConfirm bool) (string, error) {
		prompt := promptui.Prompt{
			Label:     label,
			IsConfirm: isConfirm,
		}
		return prompt.Run()
	}
	exitFunc = os.Exit
)

var rootCmd = &cobra.Command{
	Use:   "ccs",
	Short: "Claude Code Switcher",
	Long:  "ccs manages multiple Claude Code settings and switches between them safely.",
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitFunc(1)
	}
}

func init() {
	rootCmd.AddCommand(listCommand())
	rootCmd.AddCommand(useCommand())
	rootCmd.AddCommand(saveCommand())
	rootCmd.AddCommand(pruneBackupsCommand())
}

func listCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := ccs.NewManager()
			if err != nil {
				return err
			}

			entries, err := mgr.ListSettings()
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No settings found. Use 'ccs save' to create one.")
				return nil
			}
			for _, entry := range entries {
				fmt.Fprintln(cmd.OutOrStdout(), entry.Display)
			}
			return nil
		},
	}
	return cmd
}

func useCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use [name]",
		Short: "Activate a stored settings file",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := ccs.NewManager()
			if err != nil {
				return err
			}
			if err := mgr.InitInfra(); err != nil {
				return err
			}

			var target string
			if len(args) == 1 {
				target = strings.TrimSpace(args[0])
			} else {
				names, err := availableSettings(mgr)
				if err != nil {
					return err
				}
				if len(names) == 0 {
					return fmt.Errorf("no stored settings found. Use 'ccs save' first")
				}
				_, selected, err := selectFunc("Select settings to activate", names)
				if err != nil {
					return fmt.Errorf("selection cancelled")
				}
				target = selected
			}

			if err := mgr.UseSettings(target); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Successfully switched to settings: %s\n", target)
			return nil
		},
	}
	return cmd
}

func saveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "save",
		Short: "Save the current settings.json and activate it",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := ccs.NewManager()
			if err != nil {
				return err
			}
			if err := mgr.InitInfra(); err != nil {
				return err
			}

			if _, err := os.Stat(mgr.ActiveSettingsPath()); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("settings.json not found. Nothing to save")
				}
				return err
			}

			options, err := availableSettings(mgr)
			if err != nil {
				return err
			}

			const newOption = "[New Settings]"
			items := append(options, newOption)

			_, selected, err := selectFunc("Select target to save", items)
			if err != nil {
				return fmt.Errorf("selection cancelled")
			}

			var target string
			if selected == newOption {
				for {
					value, err := promptFunc("Enter new settings name", false)
					if err != nil {
						return fmt.Errorf("input cancelled")
					}
					value = strings.TrimSpace(value)
					if valid, valErr := ccs.ValidateSettingsName(value); !valid {
						fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", valErr)
						continue
					}
					if contains(options, value) {
						fmt.Fprintf(cmd.ErrOrStderr(), "Error: Settings '%s' already exists.\n", value)
						continue
					}
					target = value
					break
				}
			} else {
				target = selected
				confirmation, err := promptFunc(fmt.Sprintf("Overwrite %s?", target), true)
				if err != nil || strings.ToLower(strings.TrimSpace(confirmation)) != "y" {
					return fmt.Errorf("overwrite cancelled")
				}
			}

			if err := mgr.SaveSettings(target); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Successfully saved and activated settings: %s\n", target)
			return nil
		},
	}
	return cmd
}

func pruneBackupsCommand() *cobra.Command {
	var olderThan string
	var force bool

	cmd := &cobra.Command{
		Use:   "prune-backups",
		Short: "Remove old backup files",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := ccs.NewManager()
			if err != nil {
				return err
			}

			dur, err := ccs.ParseRetentionInterval(olderThan)
			if err != nil {
				return err
			}

			if !force {
				confirmation, err := promptFunc(fmt.Sprintf("Delete backups older than %s?", olderThan), true)
				if err != nil || strings.ToLower(strings.TrimSpace(confirmation)) != "y" {
					return fmt.Errorf("prune cancelled")
				}
			}

			removed, err := mgr.PruneBackups(dur)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %d backup file(s).\n", removed)
			return nil
		},
	}

	cmd.Flags().StringVar(&olderThan, "older-than", "30d", "Age threshold for pruning backups (e.g. 30d, 12h)")
	cmd.Flags().BoolVar(&force, "force", false, "Do not prompt for confirmation")

	return cmd
}

func availableSettings(mgr *ccs.Manager) ([]string, error) {
	entries, err := os.ReadDir(mgr.SettingsStoreDir())
	if err != nil {
		return nil, err
	}
	names := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if name := entry.Name(); strings.HasSuffix(name, ".json") {
			names = append(names, strings.TrimSuffix(name, ".json"))
		}
	}
	sort.Strings(names)
	return names, nil
}

func contains(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}
