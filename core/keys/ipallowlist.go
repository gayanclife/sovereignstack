// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package keys

import (
	"net"
	"strings"
)

// IsIPAllowed returns true when source is permitted to authenticate as this
// profile. The allowlist only applies to service accounts (Role=="service")
// with a non-empty IPAllowlist; everything else is unrestricted.
//
// source may be a bare IPv4/IPv6 ("203.0.113.5", "2001:db8::1") or include
// a port ("203.0.113.5:54321"). Any unparseable input is rejected as a
// safety default.
//
// Allowlist entries may be:
//   - exact IPs: "203.0.113.5"
//   - CIDR ranges: "10.0.0.0/8", "172.16.0.0/12", "fd00::/8"
//
// Unrecognised entries are skipped (best-effort match) — they will never
// match anything and so amount to "deny", which is the safe failure mode.
func (p *UserProfile) IsIPAllowed(source string) bool {
	if p.Role != RoleService {
		return true
	}
	if len(p.IPAllowlist) == 0 {
		return true
	}

	host := source
	if h, _, err := net.SplitHostPort(source); err == nil {
		host = h
	}
	host = strings.TrimSpace(host)
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	for _, entry := range p.IPAllowlist {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			_, cidr, err := net.ParseCIDR(entry)
			if err != nil {
				continue
			}
			if cidr.Contains(ip) {
				return true
			}
		} else {
			if rhs := net.ParseIP(entry); rhs != nil && rhs.Equal(ip) {
				return true
			}
		}
	}
	return false
}
