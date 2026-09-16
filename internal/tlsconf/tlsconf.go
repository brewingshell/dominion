// Package tlsconf provisions a local certificate authority and a leaf
// certificate for dominion. The CA is generated once and persisted, so its
// fingerprint is stable across restarts; the leaf is regenerated on each start
// with the host's current addresses.
package tlsconf

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Files holds the paths to the generated material.
type Files struct {
	CAFile   string
	CertFile string
	KeyFile  string
	// CAFingerprint is the hex-encoded SHA-256 of the CA certificate (DER).
	CAFingerprint string
}

// Ensure loads or creates the CA in dir and writes a fresh leaf certificate
// valid for the host's addresses plus extraSANs. It returns the resulting
// paths and the CA fingerprint.
func Ensure(dir string, extraSANs []string) (Files, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Files{}, fmt.Errorf("create tls dir: %w", err)
	}
	files := Files{
		CAFile:   filepath.Join(dir, "ca.pem"),
		CertFile: filepath.Join(dir, "cert.pem"),
		KeyFile:  filepath.Join(dir, "key.pem"),
	}

	caCert, caKey, err := loadOrCreateCA(filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca-key.pem"))
	if err != nil {
		return Files{}, err
	}
	sum := sha256.Sum256(caCert.Raw)
	files.CAFingerprint = hex.EncodeToString(sum[:])

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Files{}, fmt.Errorf("generate leaf key: %w", err)
	}
	leaf, err := newLeaf(caCert, caKey, leafKey, currentSANs(extraSANs))
	if err != nil {
		return Files{}, err
	}
	keyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		return Files{}, fmt.Errorf("marshal leaf key: %w", err)
	}
	if err := writeFile(files.CertFile, 0o644, pemBlock("CERTIFICATE", leaf)); err != nil {
		return Files{}, err
	}
	if err := writeFile(files.KeyFile, 0o600, pemBlock("EC PRIVATE KEY", keyDER)); err != nil {
		return Files{}, err
	}
	return files, nil
}

// Fingerprint returns the hex SHA-256 of a PEM-encoded certificate.
func Fingerprint(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", fmt.Errorf("no PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:]), nil
}

// LoadTLS builds a tls.Config from the persisted files, requesting an
// HTTP/2-capable certificate.
func LoadTLS(files Files) (tls.Certificate, error) {
	return tls.LoadX509KeyPair(files.CertFile, files.KeyFile)
}

func loadOrCreateCA(certPath, keyPath string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	if certPEM, err := os.ReadFile(certPath); err == nil {
		if keyPEM, err := os.ReadFile(keyPath); err == nil {
			cert, err := parseCert(certPEM)
			if err != nil {
				return nil, nil, err
			}
			key, err := parseECKey(keyPEM)
			if err != nil {
				return nil, nil, err
			}
			return cert, key, nil
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate CA key: %w", err)
	}
	serial, err := randSerial()
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "dominion local CA", Organization: []string{"dominion"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create CA: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal CA key: %w", err)
	}
	if err := writeFile(certPath, 0o644, pemBlock("CERTIFICATE", der)); err != nil {
		return nil, nil, err
	}
	if err := writeFile(keyPath, 0o600, pemBlock("EC PRIVATE KEY", keyDER)); err != nil {
		return nil, nil, err
	}
	caCert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return caCert, key, nil
}

func newLeaf(ca *x509.Certificate, caKey *ecdsa.PrivateKey, key *ecdsa.PrivateKey, san []string) ([]byte, error) {
	serial, err := randSerial()
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "dominion"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, host := range san {
		if ip := net.ParseIP(host); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, host)
		}
	}
	return x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
}

// currentSANs gathers the host's non-loopback IPs, hostname, localhost, and any
// caller-supplied names.
func currentSANs(extra []string) []string {
	san := []string{"localhost", "127.0.0.1", "::1"}
	seen := map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true}
	add := func(h string) {
		h = strings.TrimSpace(h)
		if h == "" || seen[h] {
			return
		}
		seen[h] = true
		san = append(san, h)
	}
	if host, err := os.Hostname(); err == nil {
		add(host)
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				add(ipnet.IP.String())
			}
		}
	}
	for _, h := range extra {
		add(h)
	}
	return san
}

func parseCert(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block in certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}

func parseECKey(pemBytes []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block in key")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

func randSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	return rand.Int(rand.Reader, limit)
}

func pemBlock(typ string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
}

func writeFile(path string, perm os.FileMode, data []byte) error {
	if err := os.WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
