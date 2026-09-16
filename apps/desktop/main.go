// Command dominion-desktop is a thin native shell around the dominion portal.
//
// It embeds the address prompt (the same www/ page the Android app uses) and
// loads the server UI in the system WebView (WebKitGTK), so there is no bundled
// browser: the binary is a few megabytes instead of Electron's ~170 MB.
//
// www/ is a synced copy of ../../www (go:embed cannot reach outside the module);
// keep it in step with apps/build-desktop.sh or `diff` against it.
//
// webview_go asks pkg-config for webkit2gtk-4.0, which Debian 13/Ubuntu 24.04 do
// not ship (only 4.1). Generate a shim before building:
//
//	go generate ./...   # runs gen-pkgconfig.sh
//	PKG_CONFIG_PATH=$PWD/.pkgconfig go build .
//
// The server address is remembered in ~/.config/dominion/client.json. On launch
// the shell probes it and, if reachable, goes straight to the portal; otherwise
// it shows the prompt with an error. "Change server" on the login screen calls
// back into this program, which re-shows the prompt.
package main

//go:generate ./gen-pkgconfig.sh

import (
	"crypto/tls"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	webview "github.com/webview/webview_go"
)

//go:embed www/index.html
var indexHTML string

//go:embed www/bootstrap.js
var bootstrapJS string

type config struct {
	Host   string `json:"host"`
	Scheme string `json:"scheme"`
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "dominion", "client.json")
}

func loadConfig() config {
	var c config
	b, err := os.ReadFile(configPath())
	if err != nil {
		return c
	}
	_ = json.Unmarshal(b, &c)
	if c.Scheme == "" {
		c.Scheme = "http"
	}
	return c
}

func saveConfig(c config) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// reachable reports whether the portal answers /healthz. Certificate errors are
// ignored here: this is a reachability check, not a security decision, and the
// WebView enforces trust when it actually loads the page.
func reachable(scheme, host string) bool {
	client := &http.Client{
		Timeout: 6 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	res, err := client.Get(fmt.Sprintf("%s://%s/healthz", scheme, host))
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode >= 200 && res.StatusCode < 400
}

// promptHTML inlines bootstrap.js and the remembered values into the prompt.
func promptHTML(saved config, errMsg string) string {
	savedJSON, _ := json.Marshal(map[string]string{
		"host":   saved.Host,
		"scheme": saved.Scheme,
	})
	inline := "<script>window.__dominionSaved=" + string(savedJSON) + ";"
	if errMsg != "" {
		msgJSON, _ := json.Marshal(errMsg)
		inline += "window.__dominionLoadError=" + string(msgJSON) + ";"
	}
	inline += "</script>\n<script>" + bootstrapJS + "</script>"

	html := strings.Replace(indexHTML,
		`<script src="bootstrap.js"></script>`,
		inline, 1)
	return html
}

func main() {
	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("dominion")
	w.SetSize(1100, 720, webview.HintNone)

	showPrompt := func(errMsg string) {
		w.SetHtml(promptHTML(loadConfig(), errMsg))
	}

	// Called by the prompt: probe, persist, then load the portal.
	if err := w.Bind("dominionConnect", func(host, scheme string) (string, error) {
		if !reachable(scheme, host) {
			return "", fmt.Errorf("could not reach %s://%s", scheme, host)
		}
		if err := saveConfig(config{Host: host, Scheme: scheme}); err != nil {
			return "", err
		}
		w.Navigate(fmt.Sprintf("%s://%s/", scheme, host))
		return "ok", nil
	}); err != nil {
		log.Fatalf("bind dominionConnect: %v", err)
	}

	// Called by "Change server" on the login screen.
	if err := w.Bind("dominionChangeURL", func() (string, error) {
		w.Dispatch(func() { showPrompt("") })
		return "ok", nil
	}); err != nil {
		log.Fatalf("bind dominionChangeURL: %v", err)
	}

	cfg := loadConfig()
	switch {
	case cfg.Host == "":
		showPrompt("")
	case reachable(cfg.Scheme, cfg.Host):
		w.Navigate(fmt.Sprintf("%s://%s/", cfg.Scheme, cfg.Host))
	default:
		showPrompt(fmt.Sprintf("Could not reach %s://%s. Check the address and that the server is running; otherwise change the server below.", cfg.Scheme, cfg.Host))
	}

	w.Run()
}
