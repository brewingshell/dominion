package tlsconf

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureGeneratesUsableCert(t *testing.T) {
	dir := t.TempDir()
	files, err := Ensure(dir, []string{"example.test", "10.1.2.3"})
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if files.CAFingerprint == "" {
		t.Fatal("missing CA fingerprint")
	}

	// The key pair must load and chain to the CA.
	pair, err := LoadTLS(files)
	if err != nil {
		t.Fatalf("LoadTLS: %v", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	ca := readCert(t, files.CAFile)
	if err := leaf.CheckSignatureFrom(ca); err != nil {
		t.Fatalf("leaf not signed by CA: %v", err)
	}

	// SANs must include localhost plus the requested names.
	names := map[string]bool{}
	for _, d := range leaf.DNSNames {
		names[d] = true
	}
	for _, ip := range leaf.IPAddresses {
		names[ip.String()] = true
	}
	if !names["localhost"] {
		t.Error("leaf should cover localhost")
	}
	if !names["example.test"] {
		t.Error("leaf should include the extra DNS SAN")
	}
	if !names["10.1.2.3"] {
		t.Error("leaf should include the extra IP SAN")
	}
	if err := leaf.VerifyHostname("10.1.2.3"); err != nil {
		t.Errorf("VerifyHostname(ip): %v", err)
	}

	// Key file must be private.
	if info, err := os.Stat(files.KeyFile); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Errorf("key perm = %v, want 0600", info.Mode().Perm())
	}
}

func TestCAIsStableAcrossRuns(t *testing.T) {
	dir := t.TempDir()
	first, err := Ensure(dir, nil)
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	second, err := Ensure(dir, nil)
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if first.CAFingerprint != second.CAFingerprint {
		t.Fatalf("CA fingerprint changed between runs: %s != %s", first.CAFingerprint, second.CAFingerprint)
	}
}

func TestLeafRegeneratesForNewSAN(t *testing.T) {
	dir := t.TempDir()
	first, err := Ensure(dir, []string{"one.test"})
	if err != nil {
		t.Fatal(err)
	}
	firstPEM, _ := os.ReadFile(first.CertFile)

	second, err := Ensure(dir, []string{"two.test"})
	if err != nil {
		t.Fatal(err)
	}
	secondPEM, _ := os.ReadFile(second.CertFile)
	if string(firstPEM) == string(secondPEM) {
		t.Error("leaf should be regenerated when SANs are supplied")
	}
	leaf := parseCertPEM(t, secondPEM)
	found := false
	for _, d := range leaf.DNSNames {
		if d == "two.test" {
			found = true
		}
	}
	if !found {
		t.Error("regenerated leaf missing the new SAN")
	}
}

func TestFingerprint(t *testing.T) {
	dir := t.TempDir()
	files, err := Ensure(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	certPEM, _ := os.ReadFile(files.CAFile)
	fp, err := Fingerprint(certPEM)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if fp != files.CAFingerprint {
		t.Fatalf("fingerprint mismatch: %s != %s", fp, files.CAFingerprint)
	}
}

func TestTLSHandshakeServesLeaf(t *testing.T) {
	dir := t.TempDir()
	files, err := Ensure(dir, []string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := LoadTLS(files)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &tls.Config{Certificates: []tls.Certificate{pair}}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	pool := x509.NewCertPool()
	pool.AddCert(readCert(t, files.CAFile))
	done := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		done <- conn.(*tls.Conn).Handshake()
	}()

	dialer := &tls.Config{RootCAs: pool, ServerName: "127.0.0.1"}
	client, err := tls.Dial("tcp", ln.Addr().String(), dialer)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	client.Close()
	if err := <-done; err != nil {
		t.Fatalf("server handshake: %v", err)
	}
}

func readCert(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return parseCertPEM(t, b)
}

func parseCertPEM(t *testing.T, b []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(b)
	if block == nil {
		t.Fatal("no PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

// ensure the host address helper is not accidentally empty on this machine.
func TestCurrentSANsIncludesLoopbackName(t *testing.T) {
	sans := currentSANs(nil)
	found := false
	for _, s := range sans {
		if s == "localhost" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected localhost in SANs, got %v", sans)
	}
}

func TestKeyFileDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "tls")
	if _, err := Ensure(dir, nil); err != nil {
		t.Fatalf("Ensure should create the directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ca.pem")); err != nil {
		t.Fatalf("ca.pem not created: %v", err)
	}
}
