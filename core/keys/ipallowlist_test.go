// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package keys

import "testing"

func TestIsIPAllowed_NonServiceRoleAlwaysAllowed(t *testing.T) {
	p := &UserProfile{Role: RoleUser, IPAllowlist: []string{"10.0.0.0/8"}}
	if !p.IsIPAllowed("203.0.113.5") {
		t.Error("IPAllowlist must not apply to non-service roles")
	}
}

func TestIsIPAllowed_EmptyAllowlistAllowsAny(t *testing.T) {
	p := &UserProfile{Role: RoleService}
	if !p.IsIPAllowed("203.0.113.5") {
		t.Error("empty allowlist on service role must allow any IP")
	}
}

func TestIsIPAllowed_ExactIPMatch(t *testing.T) {
	p := &UserProfile{Role: RoleService, IPAllowlist: []string{"203.0.113.5"}}
	if !p.IsIPAllowed("203.0.113.5") {
		t.Error("exact IP should match")
	}
	if p.IsIPAllowed("203.0.113.6") {
		t.Error("different IP should be rejected")
	}
}

func TestIsIPAllowed_CIDRMatch(t *testing.T) {
	p := &UserProfile{Role: RoleService, IPAllowlist: []string{"10.0.0.0/8"}}
	if !p.IsIPAllowed("10.5.6.7") {
		t.Error("IP inside CIDR should match")
	}
	if p.IsIPAllowed("11.0.0.1") {
		t.Error("IP outside CIDR should be rejected")
	}
}

func TestIsIPAllowed_MultipleEntries(t *testing.T) {
	p := &UserProfile{Role: RoleService, IPAllowlist: []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"203.0.113.5",
	}}

	cases := []struct {
		ip    string
		allow bool
	}{
		{"10.1.2.3", true},
		{"172.16.5.5", true},
		{"172.31.0.0", true},
		{"203.0.113.5", true},
		{"203.0.113.6", false},
		{"192.168.1.1", false},
	}
	for _, c := range cases {
		if got := p.IsIPAllowed(c.ip); got != c.allow {
			t.Errorf("%s: got %v, want %v", c.ip, got, c.allow)
		}
	}
}

func TestIsIPAllowed_StripsPort(t *testing.T) {
	p := &UserProfile{Role: RoleService, IPAllowlist: []string{"10.0.0.0/8"}}
	if !p.IsIPAllowed("10.5.6.7:54321") {
		t.Error("host:port form should strip port and match IP")
	}
}

func TestIsIPAllowed_RejectsMalformed(t *testing.T) {
	p := &UserProfile{Role: RoleService, IPAllowlist: []string{"10.0.0.0/8"}}
	for _, in := range []string{"not-an-ip", "", "999.999.999.999"} {
		if p.IsIPAllowed(in) {
			t.Errorf("malformed input %q should be rejected", in)
		}
	}
}

func TestIsIPAllowed_IPv6CIDR(t *testing.T) {
	p := &UserProfile{Role: RoleService, IPAllowlist: []string{"fd00::/8"}}
	if !p.IsIPAllowed("fd00:1::1") {
		t.Error("IPv6 in CIDR should match")
	}
	if p.IsIPAllowed("2001:db8::1") {
		t.Error("IPv6 outside CIDR should be rejected")
	}
}

func TestIsIPAllowed_SkipsBadEntriesNotMatchAll(t *testing.T) {
	p := &UserProfile{Role: RoleService, IPAllowlist: []string{
		"not-a-cidr/garbage",
		"10.0.0.0/8",
	}}
	// Bad entry is skipped; the second entry still matches.
	if !p.IsIPAllowed("10.5.6.7") {
		t.Error("valid entry should still match when bad entry is skipped")
	}
	// Bad entry alone never matches anything.
	q := &UserProfile{Role: RoleService, IPAllowlist: []string{"not-a-cidr/garbage"}}
	if q.IsIPAllowed("10.5.6.7") {
		t.Error("only bad entries should result in deny")
	}
}
