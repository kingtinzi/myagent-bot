// PinchBot Launcher - Standalone HTTP service
//
// Provides a web-based JSON editor for picoclaw config files,
// with OAuth provider authentication support.
//
// Usage:
//
//	go build -o pinchbot-launcher ./cmd/picoclaw-launcher/
//	./pinchbot-launcher [config.json]
//	./pinchbot-launcher -public config.json

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	launcherui "github.com/sipeed/pinchbot/pkg/launcherui"
)

func main() {
	public := flag.Bool("public", false, "Listen on all interfaces (0.0.0.0) instead of localhost only")
	publicUser := flag.String("public-user", "", "Username required when -public is enabled (or set PICOCLAW_PUBLIC_USER)")
	publicPass := flag.String("public-pass", "", "Password required when -public is enabled (or set PICOCLAW_PUBLIC_PASS)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "PinchBot Launcher - A web-based configuration editor\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [config.json]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Arguments:\n")
		fmt.Fprintf(os.Stderr, "  config.json    Path to the configuration file (default: .openclaw/config.json beside the executable)\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s                          Use default config path\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s ./config.json             Specify a config file\n", os.Args[0])
		fmt.Fprintf(
			os.Stderr,
			"  %s -public -public-user admin -public-pass secret ./config.json\n",
			os.Args[0],
		)
	}
	flag.Parse()

	configPath := launcherui.DefaultConfigPath()
	if flag.NArg() > 0 {
		configPath = flag.Arg(0)
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		log.Fatalf("Failed to resolve config path: %v", err)
	}

	var addr string
	if *public {
		addr = "0.0.0.0:" + launcherui.DefaultPort
	} else {
		addr = "127.0.0.1:" + launcherui.DefaultPort
	}
	authCfg := launcherui.PublicAuthConfig{
		Username: firstNonEmpty(*publicUser, os.Getenv("PICOCLAW_PUBLIC_USER")),
		Password: firstNonEmpty(*publicPass, os.Getenv("PICOCLAW_PUBLIC_PASS")),
	}
	if *public && !authCfg.Valid() {
		log.Fatal("public mode requires both -public-user/-public-pass or PICOCLAW_PUBLIC_USER/PICOCLAW_PUBLIC_PASS")
	}

	handler, err := launcherui.NewHandler(absPath)
	if err != nil {
		log.Fatalf("Failed to create launcher handler: %v", err)
	}
	if *public {
		handler = launcherui.WrapWithPublicBasicAuth(handler, authCfg)
	}

	// Print startup banner
	fmt.Println("=============================================")
	fmt.Println("  PinchBot Launcher")
	fmt.Println("=============================================")
	fmt.Printf("  Config file : %s\n", absPath)
	fmt.Printf("  Listen addr : %s\n\n", addr)
	fmt.Println("  Open the following URL in your browser")
	fmt.Println("  to view and edit the configuration:")
	fmt.Println()
	fmt.Printf("    >> http://localhost:%s <<\n", launcherui.DefaultPort)
	if *public {
		if ip := launcherui.GetLocalIP(); ip != "" {
			fmt.Printf("    >> http://%s:%s <<\n", ip, launcherui.DefaultPort)
		}
		fmt.Println("  Public mode is protected with HTTP Basic Auth.")
	}
	fmt.Println()
	// fmt.Println("=============================================")

	go func() {
		// Wait briefly to ensure the server is ready before opening the browser
		time.Sleep(500 * time.Millisecond)
		url := "http://localhost:" + launcherui.DefaultPort
		if err := openBrowser(url); err != nil {
			log.Printf("Warning: Failed to auto-open browser: %v\n", err)
		}
	}()

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// openBrowser automatically opens the given URL in the default browser.
func openBrowser(url string) error {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	return err
}
