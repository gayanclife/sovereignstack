// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

// Package tls provides TLS configuration helpers shared by every
// SovereignStack service. The core idea: TLS is on by default; HTTP is an
// explicit opt-out (--insecure-http).
//
// First-run experience:
//
//  1. Operator starts a service with no --tls-cert / --tls-key flags.
//  2. We auto-generate a self-signed RSA-2048 cert + key in
//     ~/.sovereignstack/tls/ (or the configured dir) if missing.
//  3. We log a SHA-256 fingerprint of the cert so the operator can pin it
//     in clients.
//  4. The service listens on HTTPS.
//
// Production:
//
//  1. Operator passes --tls-cert /etc/letsencrypt/.../fullchain.pem
//     --tls-key /etc/letsencrypt/.../privkey.pem
//  2. We use those files. No self-signed generation happens.
//
// Local dev:
//
//  1. Operator passes --insecure-http and accepts the warning.
//  2. We listen on plain HTTP.
package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Resolve returns the cert and key paths the caller should use to start
// an HTTPS server. If both are non-empty after this call, the caller
// should hand them to http.Server.ListenAndServeTLS or equivalent.
//
// Behaviour:
//   - If insecureHTTP is true: returns ("", "", nil) — caller should listen
//     on plain HTTP. The caller is responsible for printing a warning.
//   - If certPath and keyPath are both set and the files exist: returns
//     them as-is (production case).
//   - Otherwise: ensures dir exists, generates a self-signed cert + key
//     pair into <dir>/cert.pem and <dir>/key.pem (if missing), and
//     returns those paths. fingerprint is printable with "%s".
//
// hostnames is the SAN list to embed in self-signed certs. Pass at minimum
// "localhost" and any IPs/hostnames clients will use. Ignored when
// caller-supplied certs are used.
func Resolve(certPath, keyPath, dir string, insecureHTTP bool, hostnames []string) (cert, key, fingerprint string, err error) {
	if insecureHTTP {
		return "", "", "", nil
	}

	if certPath != "" && keyPath != "" {
		if _, err := os.Stat(certPath); err != nil {
			return "", "", "", fmt.Errorf("--tls-cert %q: %w", certPath, err)
		}
		if _, err := os.Stat(keyPath); err != nil {
			return "", "", "", fmt.Errorf("--tls-key %q: %w", keyPath, err)
		}
		fp, _ := certFingerprint(certPath)
		return certPath, keyPath, fp, nil
	}

	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".sovereignstack", "tls")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", "", fmt.Errorf("create tls dir: %w", err)
	}
	cert = filepath.Join(dir, "cert.pem")
	key = filepath.Join(dir, "key.pem")

	// Generate only if missing.
	_, certErr := os.Stat(cert)
	_, keyErr := os.Stat(key)
	if certErr != nil || keyErr != nil {
		if err := generateSelfSigned(cert, key, hostnames); err != nil {
			return "", "", "", fmt.Errorf("generate self-signed: %w", err)
		}
	}
	fp, err := certFingerprint(cert)
	if err != nil {
		return "", "", "", fmt.Errorf("read fingerprint: %w", err)
	}
	return cert, key, fp, nil
}

// generateSelfSigned writes an ECDSA-P256 cert + key pair to certPath/keyPath.
// The cert is valid for 1 year, with hostnames as DNS SANs and any IP-shaped
// hostnames recognised as IP SANs.
func generateSelfSigned(certPath, keyPath string, hostnames []string) error {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	serialMax := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialMax)
	if err != nil {
		return err
	}

	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "sovereignstack-self-signed"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:         true,
		BasicConstraintsValid: true,
	}
	if len(hostnames) == 0 {
		hostnames = []string{"localhost"}
	}
	for _, h := range hostnames {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	certOut, err := os.OpenFile(certPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		return err
	}

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return err
	}
	keyOut, err := os.OpenFile(keyPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	return pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

// certFingerprint returns the SHA-256 fingerprint of the leaf certificate
// at path, formatted like "sha256:aa:bb:cc:..." for clipboard-friendly
// pinning in clients.
func certFingerprint(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return "", fmt.Errorf("no PEM block in %s", path)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(cert.Raw)
	hex := hex.EncodeToString(sum[:])
	// Insert colons every two chars for human-readability.
	out := make([]byte, 0, len(hex)+len(hex)/2+len("sha256:"))
	out = append(out, "sha256:"...)
	for i := 0; i < len(hex); i += 2 {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, hex[i], hex[i+1])
	}
	return string(out), nil
}
