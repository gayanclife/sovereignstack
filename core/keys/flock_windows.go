// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

//go:build windows

package keys

import "os"

// flockExclusive on Windows is a no-op. Concurrent in-process writers are
// still serialised by the KeyStore mutex; cross-process synchronisation
// would require Windows-native LockFileEx, which we may add later if a
// production user needs it. For now the rename-based write still keeps
// the on-disk file consistent — only ordering of concurrent writers may
// surprise callers on Windows.
func flockExclusive(f *os.File) error { return nil }

func flockUnlock(f *os.File) {}
