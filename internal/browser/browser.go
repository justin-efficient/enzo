// Package browser opens a URL in whatever the user reads the web with.
package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Open shows url in the user's default browser and returns as soon as the
// browser has been launched, without waiting for it to close.
func Open(url string) error {
	name, args := command(runtime.GOOS, url)
	if name == "" {
		return fmt.Errorf("no browser opener known for %s", runtime.GOOS)
	}

	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	// Reap the opener without blocking the caller. What the browser does
	// afterwards is not enzo's business.
	go func() { _ = cmd.Wait() }()
	return nil
}

// command returns the platform's "open this URL" command. goos is a parameter
// rather than read from runtime so every branch is testable on one machine.
func command(goos, url string) (name string, args []string) {
	switch goos {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "linux", "freebsd", "netbsd", "openbsd", "dragonfly", "solaris":
		// xdg-open honours $BROWSER and the desktop's default, so enzo does
		// not need to know either.
		return "xdg-open", []string{url}
	default:
		return "", nil
	}
}
