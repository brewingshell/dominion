package main

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/brewingshell/dominion/internal/tlsconf"
)

// startMixed serves handler on a peekListener that answers both HTTP and HTTPS.
func startMixed(t *testing.T, handler http.Handler) (addr string, pool *x509.CertPool) {
	t.Helper()

	files, err := tlsconf.Ensure(t.TempDir(), []string{"127.0.0.1", "localhost"})
	if err != nil {
		t.Fatalf("tlsconf: %v", err)
	}
	cert, err := tls.LoadX509KeyPair(files.CertFile, files.KeyFile)
	if err != nil {
		t.Fatalf("load keypair: %v", err)
	}
	pool = x509.NewCertPool()
	caPEM, err := os.ReadFile(files.CAFile)
	if err != nil {
		t.Fatal(err)
	}
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("failed to add CA to pool")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	peek := &peekListener{Listener: ln, tlsConfig: &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"http/1.1"},
	}}

	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(peek) }()
	t.Cleanup(func() { _ = srv.Close() })

	return ln.Addr().String(), pool
}

func TestPeekListenerServesHTTPAndHTTPS(t *testing.T) {
	addr, pool := startMixed(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "scheme="+r.URL.Scheme+" tls="+boolStr(r.TLS != nil))
	}))

	// Plain HTTP on the same port.
	res, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || string(body) != "scheme= tls=false" {
		t.Fatalf("http: status=%d body=%q", res.StatusCode, body)
	}

	// HTTPS on the same port, verified against the generated CA.
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	res, err = client.Get("https://" + addr + "/")
	if err != nil {
		t.Fatalf("https get: %v", err)
	}
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || string(body) != "scheme= tls=true" {
		t.Fatalf("https: status=%d body=%q", res.StatusCode, body)
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
