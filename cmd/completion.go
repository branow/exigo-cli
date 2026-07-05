package cmd

import (
	"github.com/spf13/cobra"
)

// newCompletionCmd builds "exigo completion", generating shell completion
// scripts via Cobra's built-in generators.
func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:                   "completion [bash|zsh|fish|powershell]",
		Short:                 "Generate shell completion scripts",
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Example: `  exigo completion bash > /etc/bash_completion.d/exigo
  exigo completion zsh > "${fpath[1]}/_exigo"
  source <(exigo completion bash)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return generateCompletion(cmd, args[0])
		},
	}
}

func generateCompletion(cmd *cobra.Command, shell string) error {
	root := cmd.Root()
	out := cmd.OutOrStdout()
	switch shell {
	case "bash":
		return root.GenBashCompletion(out)
	case "zsh":
		return root.GenZshCompletion(out)
	case "fish":
		return root.GenFishCompletion(out, true)
	default:
		return root.GenPowerShellCompletionWithDesc(out)
	}
}
