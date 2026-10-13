package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	configPath string
	jsonOut    bool
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "apismith",
		Short:         "OpenAPI-driven API testing",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long:          rootHelp,
		Example:       rootExamples,
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "environments.yaml path (default config/environments.yaml)")
	cmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output")
	cmd.AddCommand(newUICmd(), newJWTCmd(), newLSCmd(), newCallCmd(), newRunCmd())
	return cmd
}

// Execute runs the apismith CLI using os.Args.
func Execute() error {
	err := newRootCmd().Execute()
	if err == nil || errors.Is(err, errSilent) {
		return err
	}
	fmt.Fprintln(os.Stderr, "apismith:", err)
	return err
}
