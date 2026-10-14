package updater

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// OpenURL opens an https URL in the user's default browser and returns
// without waiting. It refuses anything that is not https.
func OpenURL(u string) error {
	if !strings.HasPrefix(u, "https://") {
		return fmt.Errorf("updater: refusing to open %q", u)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "js":
		return ErrUnsupported
	default:
		cmd = exec.Command("xdg-open", u)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("updater: opening the browser: %w", err)
	}
	go cmd.Wait()
	return nil
}
