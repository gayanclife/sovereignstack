// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Build metadata. Populated by main.SetVersionInfo at startup so this
// package doesn't need to know about ldflags injection — main.go owns
// that contract.
var (
	buildVersion = "dev"
	buildCommit  = "none"
	buildDate    = "unknown"
)

// SetVersionInfo is called from main.go with the values injected by
// GoReleaser via `-ldflags "-X main.version=... -X main.commit=..."`.
func SetVersionInfo(version, commit, date string) {
	if version != "" {
		buildVersion = version
	}
	if commit != "" {
		buildCommit = commit
	}
	if date != "" {
		buildDate = date
	}
	rootCmd.Version = buildVersion
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version, commit, build date, and Go runtime info",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return emit(cmd, map[string]any{
			"version":    buildVersion,
			"commit":     buildCommit,
			"build_date": buildDate,
			"go":         runtime.Version(),
			"os":         runtime.GOOS,
			"arch":       runtime.GOARCH,
		}, func() {
			fmt.Printf("sovstack %s\n", buildVersion)
			fmt.Printf("  commit:     %s\n", buildCommit)
			fmt.Printf("  built:      %s\n", buildDate)
			fmt.Printf("  go runtime: %s (%s/%s)\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
		})
	},
}

func init() {
	versionCmd.Flags().StringP("output", "o", "text", "Output format: text | json")
	rootCmd.AddCommand(versionCmd)
}
