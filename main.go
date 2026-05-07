/*
Copyright 2026 SovereignStack Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
...
*/

package main

import (
	"github.com/gayanclife/sovereignstack/cmd"
)

// Build metadata, populated by GoReleaser at link time:
//
//	go build -ldflags "-X main.version=v1.0.0 -X main.commit=abc1234 -X main.date=2026-05-07"
//
// `cmd.Execute` reads these via cmd.SetVersionInfo so the `version`
// subcommand can report them.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetVersionInfo(version, commit, date)
	cmd.Execute()
}
