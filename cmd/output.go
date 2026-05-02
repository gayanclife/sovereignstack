// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// outputFormat returns the value of the --output flag (defaults to "text").
// Subcommands consult this to decide between human-readable and JSON output.
func outputFormat(cmd *cobra.Command) string {
	out, _ := cmd.Flags().GetString("output")
	if out == "" {
		return "text"
	}
	return out
}

// emit either renders the payload as JSON to stdout (when --output=json) or
// invokes textRenderer for the human-friendly path. JSON output goes
// through encoding/json with a trailing newline so it's pipeable into jq.
//
// Use this in any subcommand that produces reportable output, so adding
// JSON support is a one-line change wherever it's missing.
func emit(cmd *cobra.Command, payload any, textRenderer func()) error {
	switch outputFormat(cmd) {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	case "text", "":
		textRenderer()
		return nil
	default:
		return fmt.Errorf("--output: unknown value %q (want text or json)", outputFormat(cmd))
	}
}
