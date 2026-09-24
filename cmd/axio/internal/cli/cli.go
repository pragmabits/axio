// Package cli builds the axio command line: a root command that holds what
// every command shares, and one subcommand per task.
package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// logsGroup groups the commands that read axio logs in the help.
const logsGroup = "logs"

// options holds what every command shares: the persistent flags and what the
// root resolves from them before any command runs.
type options struct {
	color        colorMode
	colorEnabled bool
}

// NewRootCommand returns the axio command with every subcommand attached.
// Output, error output and input come from the command, so a caller — a test,
// or main — sets them with SetOut, SetErr and SetIn.
func NewRootCommand() *cobra.Command {
	shared := &options{color: colorAuto}

	root := &cobra.Command{
		Use:   "axio",
		Short: "Tools for reading axio logs",
		Long: `axio reads the logs written by the axio logging library.

Logs meant for machines are JSON; axio render turns them back into the text a
person reads on a terminal, the same text the library's Console output writes.`,
		Version:       version(),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(command *cobra.Command, _ []string) {
			shared.colorEnabled = shared.color.enabled(command.OutOrStdout())
		},
	}
	root.SetVersionTemplate("axio version {{.Version}}\n")
	root.SetFlagErrorFunc(pointToHelp)
	root.AddGroup(&cobra.Group{ID: logsGroup, Title: "Reading logs:"})

	root.PersistentFlags().Var(&shared.color, "color", "when to color the output: auto, always or never")
	// Registration fails only for a flag that does not exist, and the flag is
	// defined on the line above.
	_ = root.RegisterFlagCompletionFunc("color", cobra.FixedCompletions([]string{
		"auto\tcolor only when writing to a terminal, unless NO_COLOR is set",
		"always\tcolor even when writing to a pipe or a file",
		"never\tnever color",
	}, cobra.ShellCompDirectiveNoFileComp))

	root.AddCommand(newRenderCommand(shared), newVerifyCommand())
	return root
}

// pointToHelp adds, to a flag error, the command that explains the flags.
func pointToHelp(command *cobra.Command, err error) error {
	return fmt.Errorf("%w\nRun '%s --help' for usage", err, command.CommandPath())
}

// version returns the module version the go command recorded in the binary.
// Installed with go install, it is the release, without the cmd/axio/ tag
// prefix: v0.1.0 for the tag cmd/axio/v0.1.0. Built from a checkout, it is the
// version git gives: the release at a clean tagged commit, a pseudo-version
// after it, with "+dirty" for uncommitted changes. A build that records no
// version, as go run does, reports "(devel)".
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}
