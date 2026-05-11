package commands

import (
	"fmt"
	"os"

	"github.com/legendaryos/builder/src/ui"
	"github.com/spf13/cobra"
)

var (
	flagVerbose bool
	flagRelease bool
	flagFast    bool
)

var rootCmd = &cobra.Command{
	Use:   "legendaryos",
	Short: "LegendaryOS Builder — Fedora Edition",
	Long:  `LegendaryOS Builder: build bootable Fedora-based OS images and bootc containers.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Don't print banner for completion or help
		if cmd.Name() == "__complete" || cmd.Name() == "help" {
			return
		}
	},
}

func Execute() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.SetUsageTemplate(usageTemplate())

	if err := rootCmd.Execute(); err != nil {
		fmt.Println()
		ui.Error("%v", err)
		fmt.Println()
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().BoolVar(&flagRelease, "release", false, "Release mode (strip debug, max compression)")

	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(versionCmd)
}

func usageTemplate() string {
	return `
  Usage:
    {{.UseLine}}{{if .HasAvailableSubCommands}}
    {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

  Aliases:
    {{.NameAndAliases}}{{end}}{{if .HasAvailableSubCommands}}

  Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
    {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

  Flags:
{{.LocalFlags.FlagUsages | trimRightSpace}}{{end}}{{if .HasAvailableInheritedFlags}}

  Global Flags:
{{.InheritedFlags.FlagUsages | trimRightSpace}}{{end}}{{if .HasHelpSubCommands}}

  Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
    {{rpad .Name .NamePadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

  Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
}

func cwd() string {
	d, err := os.Getwd()
	if err != nil {
		ui.Fatal("cannot determine working directory: %v", err)
	}
	return d
}
