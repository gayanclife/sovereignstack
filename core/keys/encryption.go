// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package keys

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gayanclife/sovereignstack/core/crypto"
)

// SetMasterKey enables transparent AES-GCM encryption of sensitive
// profile fields (Department, Team, Role) on save. When set, every
// future Save / saveLocked encrypts those fields under masterKey;
// LoadKeyStore can then decrypt them via DecryptProfilesInPlace.
//
// Pass nil to disable encryption (default; pre-Phase-C5 behaviour).
//
// This is opt-in: the loader handles a mixed file (some profiles
// encrypted, some plaintext) gracefully — see DecryptProfilesInPlace.
func (ks *KeyStore) SetMasterKey(masterKey []byte) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.masterKey = masterKey
}

// MigrateEncryptFields rewrites keys.json on disk with all sensitive
// fields encrypted under the configured master key. With the master key
// set, saveLocked transparently encrypts Department/Team/Role; this
// method is a thin wrapper that triggers a save.
//
// Returns the number of profiles whose on-disk representation actually
// changed (was plaintext, now ciphertext). Re-running the migration on
// an already-encrypted file is safe and reports 0.
func (ks *KeyStore) MigrateEncryptFields() (int, error) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if len(ks.masterKey) == 0 {
		return 0, fmt.Errorf("master key not set; call SetMasterKey first")
	}

	// Read the existing on-disk file (if any) to figure out what's
	// already encrypted. A profile counts as "needs migration" when
	// its on-disk form has any plaintext sensitive field.
	var onDisk keyStoreData
	if data, err := os.ReadFile(ks.path); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &onDisk)
	}

	migrated := 0
	for id := range ks.Users {
		prev, ok := onDisk.Users[id]
		if !ok {
			// New user that hasn't hit disk yet — saveLocked will
			// encrypt on the way out, so it counts as a migration.
			migrated++
			continue
		}
		if (prev.Department != "" && !crypto.IsEncrypted(prev.Department)) ||
			(prev.Team != "" && !crypto.IsEncrypted(prev.Team)) ||
			(prev.Role != "" && !crypto.IsEncrypted(prev.Role)) {
			migrated++
		}
	}

	if err := ks.saveLocked(); err != nil {
		return migrated, fmt.Errorf("save migrated store: %w", err)
	}
	return migrated, nil
}

// DecryptProfilesInPlace walks the in-memory map after LoadKeyStore and
// replaces ciphertexts with plaintext under masterKey. Plaintext fields
// (no $enc1$ prefix) are left alone. Caller must hold ks.mu.
//
// Use this immediately after LoadKeyStore + SetMasterKey, before any
// reads. The runtime accessors (GetByID, GetByKey) all return the
// in-memory profile, so post-decryption every reader sees plaintext.
func (ks *KeyStore) DecryptProfilesInPlace(masterKey []byte) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	for _, p := range ks.Users {
		dept, err := crypto.Decrypt(masterKey, p.Department)
		if err != nil {
			return fmt.Errorf("decrypt department for %q: %w", p.ID, err)
		}
		team, err := crypto.Decrypt(masterKey, p.Team)
		if err != nil {
			return fmt.Errorf("decrypt team for %q: %w", p.ID, err)
		}
		role, err := crypto.Decrypt(masterKey, p.Role)
		if err != nil {
			return fmt.Errorf("decrypt role for %q: %w", p.ID, err)
		}
		p.Department, p.Team, p.Role = dept, team, role
	}
	return nil
}

// encryptFieldsLocked encrypts the three sensitive fields on profile p
// under masterKey. Already-encrypted fields are skipped (idempotent).
// Caller must hold the write lock.
func encryptFieldsLocked(masterKey []byte, p *UserProfile) error {
	if !crypto.IsEncrypted(p.Department) && p.Department != "" {
		ct, err := crypto.Encrypt(masterKey, p.Department)
		if err != nil {
			return err
		}
		p.Department = ct
	}
	if !crypto.IsEncrypted(p.Team) && p.Team != "" {
		ct, err := crypto.Encrypt(masterKey, p.Team)
		if err != nil {
			return err
		}
		p.Team = ct
	}
	if !crypto.IsEncrypted(p.Role) && p.Role != "" {
		ct, err := crypto.Encrypt(masterKey, p.Role)
		if err != nil {
			return err
		}
		p.Role = ct
	}
	return nil
}

// HasEncryptedFields returns true if any user profile carries an
// encrypted Department, Team, or Role. Used by the policy startup
// banner to nudge operators toward configuring a master key when one
// is required to read existing data.
func (ks *KeyStore) HasEncryptedFields() bool {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	for _, p := range ks.Users {
		if crypto.IsEncrypted(p.Department) ||
			crypto.IsEncrypted(p.Team) ||
			crypto.IsEncrypted(p.Role) {
			return true
		}
	}
	return false
}
