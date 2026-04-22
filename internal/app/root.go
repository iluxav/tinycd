package app

import (
	"github.com/spf13/cobra"
)

func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "tcd",
		Short:         "TinyCD — deploy private repos with Docker Compose",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}

	root.AddCommand(
		newInitCmd(),
		newDeployCmd(),
		newRestartCmd(),
		newStopCmd(),
		newRmCmd(),
		newLsCmd(),
		newStatusCmd(),
		newLogsCmd(),
		newUICmd(),
		newServiceCmd(),
		newAdminCmd(),
	)
	return root
}
