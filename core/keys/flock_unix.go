// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

//go:build !windows

package keys

import (
	"os"
	"syscall"
)

// flockExclusive blocks until the file is locked exclusively (LOCK_EX).
// On Unix this uses syscall.Flock, which is advisory: well-behaved
// processes (the CLI and management service) honour it; malicious or
// uncooperative processes can still read or write the file.
func flockExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

// flockUnlock releases a previously-acquired flock. Errors are ignored;
// the kernel auto-releases the lock when the file descriptor closes.
func flockUnlock(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
