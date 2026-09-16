package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"dominion/internal/server"
	"dominion/internal/tlsconf"
)

//go:embed all:web
var webFS embed.FS

// sanList collects a repeatable -tls-san flag.
type sanList []string

func (s *sanList) String() string { return strings.Join(*s, ",") }
func (s *sanList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	defaultPIN := os.Getenv("DOMINION_PIN")
	if defaultPIN == "" {
		defaultPIN = "3232"
	}
	home, _ := os.UserHomeDir()
	defaultTLSDir := filepath.Join(home, ".config", "dominion")

	var sans sanList
	var (
		addr        = flag.String("addr", ":5550", "address to listen on")
		pin         = flag.String("pin", defaultPIN, "PIN required to access the portal (env DOMINION_PIN)")
		bin         = flag.String("tmux", "tmux", "path to the tmux binary")
		ttl         = flag.Duration("ttl", 12*time.Hour, "how long a login lasts")
		useTLS      = flag.Bool("tls", true, "serve HTTPS with a local CA-signed certificate")
		tlsDir      = flag.String("tls-dir", defaultTLSDir, "directory for CA and certificate files")
		tlsCert     = flag.String("tls-cert", "", "use this certificate file instead of generating one")
		tlsKey      = flag.String("tls-key", "", "use this key file instead of generating one")
		tlsCA       = flag.String("tls-ca", "", "use this CA certificate file instead of generating one")
		fingerprint = flag.Bool("fingerprint", false, "print the CA fingerprint and exit")
		brandingDir = flag.String("branding", "assets", "directory of logo overrides (override_logo.*, logo.*)")
	)
	flag.Var(&sans, "tls-san", "extra DNS name or IP for the certificate (repeatable)")
	flag.Parse()

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
		SecureCookies: *useTLS,
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
		if *useTLS {
			log.Printf("dominion listening on %s over HTTPS (tmux: %s)", *addr, *bin)
			log.Printf("CA certificate: %s", caFile)
			if caFingerprint != "" {
				log.Printf("CA SHA-256 fingerprint: %s", caFingerprint)
			}
			if err := httpSrv.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
				errc <- err
			}
			return
		}
		log.Printf("dominion listening on %s over HTTP (tmux: %s)", *addr, *bin)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
