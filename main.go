package main

import (
	"context"
	"crypto/tls"
	"embed"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/brewingshell/dominion/internal/server"
	"github.com/brewingshell/dominion/internal/tlsconf"
)

//go:embed all:web
var webFS embed.FS

// version is the release version. It is overridden at build time with
// -ldflags "-X main.version=…"; a source build reports "dev".
var version = "dev"

// sanList collects a repeatable -tls-san flag.
type sanList []string

func (s *sanList) String() string { return strings.Join(*s, ",") }
func (s *sanList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	home, _ := os.UserHomeDir()
	defaultTLSDir := filepath.Join(home, ".config", "dominion")
	defaultPIN := pinFromEnv(home)

	var sans sanList
	var (
		addr        = flag.String("addr", ":5550", "address to listen on")
		pin         = flag.String("pin", defaultPIN, "PIN required to access the portal (DOMINION_PIN env or .env)")
		bin         = flag.String("tmux", "tmux", "path to the tmux binary")
		ttl         = flag.Duration("ttl", 12*time.Hour, "how long a login lasts")
		useTLS      = flag.Bool("tls", true, "serve HTTPS with a local CA-signed certificate")
		tlsDir      = flag.String("tls-dir", defaultTLSDir, "directory for CA and certificate files")
		tlsCert     = flag.String("tls-cert", "", "use this certificate file instead of generating one")
		tlsKey      = flag.String("tls-key", "", "use this key file instead of generating one")
		tlsCA       = flag.String("tls-ca", "", "use this CA certificate file instead of generating one")
		fingerprint = flag.Bool("fingerprint", false, "print the CA fingerprint and exit")
		brandingDir = flag.String("branding", "assets", "directory of logo overrides (override_logo.*, logo.*)")
		allowHTTP   = flag.Bool("allow-http", true, "also accept plain HTTP on the same port (less secure)")
		secureCook  = flag.Bool("secure-cookies", false, "force the Secure cookie attribute (set behind a TLS-terminating proxy)")
		showVersion = flag.Bool("version", false, "print the version and exit")
	)
	flag.Var(&sans, "tls-san", "extra DNS name or IP for the certificate (repeatable)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("dominion %s\n", version)
		return
	}

	certFile, keyFile := *tlsCert, *tlsKey
	caFile := *tlsCA
	var caFingerprint string

	if *useTLS {
		files, err := tlsconf.Ensure(*tlsDir, sans)
		if err != nil {
			log.Fatalf("tls: %v", err)
		}
		if certFile == "" {
			certFile = files.CertFile
		}
		if keyFile == "" {
			keyFile = files.KeyFile
		}
		if caFile == "" {
			caFile = files.CAFile
		}
		caFingerprint = files.CAFingerprint
		if *tlsCA != "" {
			// A caller-supplied CA overrides the generated one for display.
			if fp, err := tlsconf.Fingerprint(readFileOrDie(*tlsCA)); err == nil {
				caFingerprint = fp
			}
		}
		if *fingerprint {
			fmt.Printf("CA certificate: %s\n", caFile)
			fmt.Printf("CA SHA-256 fingerprint: %s\n", caFingerprint)
			return
		}
	} else if *fingerprint {
		log.Fatalf("-fingerprint requires -tls")
	}

	srv := server.New(server.Config{
		PIN:           *pin,
		TmuxBin:       *bin,
		TokenTTL:      *ttl,
		WebFS:         webFS,
		Logger:        log.Default(),
		SecureCookies: *secureCook,
		BrandingDir:   *brandingDir,
	})

	httpSrv := &http.Server{
		Addr:    *addr,
		Handler: srv.Handler(),
		// ReadHeaderTimeout and IdleTimeout bound slow clients without a
		// Read/WriteTimeout, which would break the hijacked WebSocket stream.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		if !*useTLS {
			log.Printf("dominion listening on %s over HTTP (tmux: %s)", *addr, *bin)
			if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errc <- err
			}
			return
		}

		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			errc <- fmt.Errorf("load certificate: %w", err)
			return
		}
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
			// WebSocket upgrades are HTTP/1.1; don't advertise h2 we don't serve.
			NextProtos: []string{"http/1.1"},
		}

		ln, err := net.Listen("tcp", *addr)
		if err != nil {
			errc <- err
			return
		}
		if *allowHTTP {
			ln = &peekListener{Listener: ln, tlsConfig: tlsConfig}
			log.Printf("dominion listening on %s over HTTPS and HTTP (tmux: %s)", *addr, *bin)
		} else {
			ln = tls.NewListener(ln, tlsConfig)
			log.Printf("dominion listening on %s over HTTPS (tmux: %s)", *addr, *bin)
		}
		log.Printf("CA certificate: %s", caFile)
		if caFingerprint != "" {
			log.Printf("CA SHA-256 fingerprint: %s", caFingerprint)
		}
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		log.Fatalf("server: %v", err)
	case <-ctx.Done():
		log.Printf("shutting down")
		srv.CloseSockets()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}
}

func readFileOrDie(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read %s: %v", path, err)
	}
	return b
}
