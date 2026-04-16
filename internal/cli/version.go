package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	Version   = "0.1.0"
	BuildDate = "unknown"
	GitCommit = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "版本信息",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("nep2midsence %s\n", Version)
		fmt.Printf("  Build Date: %s\n", BuildDate)
		fmt.Printf("  Git Commit: %s\n", GitCommit)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
