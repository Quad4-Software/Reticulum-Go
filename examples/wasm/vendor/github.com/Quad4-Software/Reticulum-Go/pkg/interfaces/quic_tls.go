// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io
//go:build !js

package interfaces

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"
)

const quicALPN = "rns"

// parsePeerKeyPin decodes a hex SPKI SHA-256 pin. Empty input yields nil, nil.
func parsePeerKeyPin(peerKey string) ([]byte, error) {
	s := strings.TrimSpace(peerKey)
	if s == "" {
		return nil, nil
	}
	s = strings.ReplaceAll(s, ":", "")
	s = strings.ReplaceAll(s, " ", "")
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("peer_key: %w", err)
	}
	if len(b) != sha256.Size {
		return nil, fmt.Errorf("peer_key: want %d bytes, got %d", sha256.Size, len(b))
	}
	return b, nil
}

// spkiSHA256 returns the SHA-256 of the certificate SubjectPublicKeyInfo.
func spkiSHA256(cert *x509.Certificate) []byte {
	if cert == nil {
		return nil
	}
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return sum[:]
}

// SPKIPinHex returns the hex-encoded SPKI SHA-256 of cert for peer_key config.
func SPKIPinHex(cert *x509.Certificate) string {
	return hex.EncodeToString(spkiSHA256(cert))
}

func verifyPeerKeyPin(rawCerts [][]byte, pin []byte) error {
	if len(pin) == 0 {
		return nil
	}
	if len(rawCerts) == 0 {
		return fmt.Errorf("peer_key: no peer certificate")
	}
	cert, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return fmt.Errorf("peer_key: parse cert: %w", err)
	}
	got := spkiSHA256(cert)
	if !bytesEqualConstant(got, pin) {
		return fmt.Errorf("peer_key: pin mismatch")
	}
	return nil
}

func bytesEqualConstant(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// generateEphemeralQUICCert creates a self-signed ECDSA P-256 certificate.
func generateEphemeralQUICCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"reticulum"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}, nil
}

// loadOrGenerateQUICCert loads PEM cert/key files or generates an ephemeral cert.
func loadOrGenerateQUICCert(certFile, keyFile string) (tls.Certificate, error) {
	certFile = strings.TrimSpace(certFile)
	keyFile = strings.TrimSpace(keyFile)
	if certFile == "" && keyFile == "" {
		return generateEphemeralQUICCert()
	}
	if certFile == "" || keyFile == "" {
		return tls.Certificate{}, fmt.Errorf("cert_file and key_file must both be set")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("load cert: %w", err)
	}
	return cert, nil
}

// leafCertificate returns the parsed leaf from a tls.Certificate.
func leafCertificate(cert tls.Certificate) (*x509.Certificate, error) {
	if len(cert.Certificate) == 0 {
		return nil, fmt.Errorf("empty certificate")
	}
	return x509.ParseCertificate(cert.Certificate[0])
}

// buildQUICClientTLS builds a mesh-friendly client tls.Config.
func buildQUICClientTLS(sni string, peerPin []byte, clientCert tls.Certificate) *tls.Config {
	cfg := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true, // #nosec G402 mesh model: pin or IFAC, not CA
		NextProtos:         []string{quicALPN},
		Certificates:       []tls.Certificate{clientCert},
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return &clientCert, nil
		},
	}
	if sni != "" {
		cfg.ServerName = sni
	} else {
		cfg.ServerName = "reticulum"
	}
	if len(peerPin) > 0 {
		pin := append([]byte(nil), peerPin...)
		// Disable tickets so VerifyPeerCertificate cannot be skipped on resume.
		cfg.SessionTicketsDisabled = true
		cfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			return verifyPeerKeyPin(rawCerts, pin)
		}
		cfg.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return fmt.Errorf("missing peer certificate")
			}
			return verifyPeerKeyPin([][]byte{cs.PeerCertificates[0].Raw}, pin)
		}
	}
	return cfg
}

// buildQUICServerTLS builds a mesh-friendly server tls.Config.
func buildQUICServerTLS(cert tls.Certificate, peerPin []byte) *tls.Config {
	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{quicALPN},
		ClientAuth:   tls.NoClientCert,
	}
	if len(peerPin) > 0 {
		pin := append([]byte(nil), peerPin...)
		cfg.ClientAuth = tls.RequireAnyClientCert
		cfg.SessionTicketsDisabled = true
		cfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			return verifyPeerKeyPin(rawCerts, pin)
		}
		cfg.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return fmt.Errorf("missing peer certificate")
			}
			return verifyPeerKeyPin([][]byte{cs.PeerCertificates[0].Raw}, pin)
		}
	}
	return cfg
}

// writePEMCertKey writes cert and key PEM files (test helper path).
func writePEMCertKey(cert tls.Certificate, certPath, keyPath string) error {
	if len(cert.Certificate) == 0 {
		return fmt.Errorf("empty certificate")
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	keyDER, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		return err
	}
	return os.WriteFile(keyPath, keyPEM, 0o600)
}
