package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"exigo-cli/internal/cmdutil"
)

// version is the exigo-cli release version. It is a plain variable rather
// than build-time ldflags injection for this iteration.
var version = "dev"

// newVersionCmd builds "exigo version".
func newVersionCmd(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print the exigo-cli version",
		Example: `  exigo version`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(f.IOStreams.Out, "exigo version %s\n", version)
			return nil
		},
	}
}
