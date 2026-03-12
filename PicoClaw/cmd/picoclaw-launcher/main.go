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
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/sipeed/picoclaw/cmd/picoclaw-launcher/internal/server"
)

//go:embed internal/ui/index.html
var staticFiles embed.FS

func main() {
	public := flag.Bool("public", false, "Listen on all interfaces (0.0.0.0) instead of localhost only")
	publicUser := flag.String("public-user", "", "Username required when -public is enabled (or set PICOCLAW_PUBLIC_USER)")
	publicPass := flag.String("public-pass", "", "Password required when -public is enabled (or set PICOCLAW_PUBLIC_PASS)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "PinchBot Launcher - A web-based configuration editor\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [config.json]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Arguments:\n")
		fmt.Fprintf(os.Stderr, "  config.json    Path to the configuration file (default: .pinchbot/config.json beside the executable)\n\n")
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

	configPath := server.DefaultConfigPath()
	if flag.NArg() > 0 {
		configPath = flag.Arg(0)
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		log.Fatalf("Failed to resolve config path: %v", err)
	}

	var addr string
	if *public {
		addr = "0.0.0.0:" + server.DefaultPort
	} else {
		addr = "127.0.0.1:" + server.DefaultPort
	}
	authCfg := server.PublicAuthConfig{
		Username: firstNonEmpty(*publicUser, os.Getenv("PICOCLAW_PUBLIC_USER")),
		Password: firstNonEmpty(*publicPass, os.Getenv("PICOCLAW_PUBLIC_PASS")),
	}
	if *public && !authCfg.Valid() {
		log.Fatal("public mode requires both -public-user/-public-pass or PICOCLAW_PUBLIC_USER/PICOCLAW_PUBLIC_PASS")
	}

	mux := http.NewServeMux()
	server.RegisterConfigAPI(mux, absPath)
	server.RegisterAuthAPI(mux, absPath)
	server.RegisterAppPlatformAPI(mux, absPath)
	server.RegisterWorkspaceAPI(mux)
	server.RegisterProcessAPI(mux, absPath)

	staticFS, err := fs.Sub(staticFiles, "internal/ui")
	if err != nil {
		log.Fatalf("Failed to create sub filesystem: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	var handler http.Handler = mux
	if *public {
		handler = server.WrapWithPublicBasicAuth(handler, authCfg)
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
	fmt.Printf("    >> http://localhost:%s <<\n", server.DefaultPort)
	if *public {
		if ip := server.GetLocalIP(); ip != "" {
			fmt.Printf("    >> http://%s:%s <<\n", ip, server.DefaultPort)
		}
		fmt.Println("  Public mode is protected with HTTP Basic Auth.")
	}
	fmt.Println()
	// fmt.Println("=============================================")

	go func() {
		// Wait briefly to ensure the server is ready before opening the browser
		time.Sleep(500 * time.Millisecond)
		url := "http://localhost:" + server.DefaultPort
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
