package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/pragmabits/axio/internal/logline"
)

// standardInput is the file name that stands for standard input.
const standardInput = "-"

// newRenderCommand returns axio render.
func newRenderCommand(shared *options) *cobra.Command {
	var utc bool

	renderCommand := &cobra.Command{
		Use:   "render [file...]",
		Short: "Turn axio JSON logs into the Console's text",
		Long: `render reads axio JSON lines and writes the text the Console output writes
for the same entries: same columns, same colors, same fields.

It reads the files in order, or standard input when there is none; "-" also
names standard input. Each line is written as soon as it is read, so it
follows a stream that never ends. Lines that are not axio entries pass
through unchanged, and anything in front of the JSON — the pod name that
kubectl logs --prefix adds, the service name docker compose adds — stays in
front of the text. Wide events show EVENT in the level column.

Timestamps are shown in the local time zone, as the Console shows them.`,
		Example: `  kubectl logs -f deploy/checkout | axio render
  kubectl logs -f -l app=checkout --prefix | axio render
  docker compose logs -f checkout | axio render
  journalctl -u checkout -o cat -f | axio render
  axio render /var/log/checkout.log
  kubectl logs deploy/checkout | axio render | grep -E 'WARN|ERROR'
  kubectl logs deploy/checkout | axio render --color=always | less -R`,
		GroupID: logsGroup,
		Args:    cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, files []string) error {
			renderer := logline.NewRenderer(logline.RenderOptions{
				Color:    shared.colorEnabled,
				Location: timeZone(utc),
			})
			return renderFiles(command, renderer, files)
		},
	}
	renderCommand.Flags().BoolVar(&utc, "utc", false, "show timestamps in UTC instead of the local time zone")

	return renderCommand
}

// renderFiles renders each file in order, or standard input when no file is given.
func renderFiles(command *cobra.Command, renderer *logline.Renderer, files []string) error {
	if len(files) == 0 {
		files = []string{standardInput}
	}
	for _, name := range files {
		if err := renderFile(command, renderer, name); err != nil {
			return err
		}
	}
	return nil
}

// renderFile renders one file, or standard input when name is "-".
func renderFile(command *cobra.Command, renderer *logline.Renderer, name string) error {
	if name == standardInput {
		return renderer.Render(command.InOrStdin(), command.OutOrStdout())
	}
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := renderer.Render(file, command.OutOrStdout()); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// timeZone returns UTC when utc is set, and the local time zone otherwise.
func timeZone(utc bool) *time.Location {
	if utc {
		return time.UTC
	}
	return time.Local
}
