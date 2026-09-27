package ghostty

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/vitruves/ghostty-config/internal/color"
)

// Live is the running terminal. Ghostty repaints the moment it receives the
// OSC colour sequences, so a palette can be judged at its true values in the
// window being edited without a single file write. The transparent titlebar
// follows OSC 11 as well, so even the top of the window previews.
type Live struct {
	mu  sync.Mutex
	tty *os.File
}

// OpenLive attaches to the controlling terminal. It returns nil when there
// is none, and every method on a nil Live is a no-op.
func OpenLive() *Live {
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return nil
	}
	return &Live{tty: tty}
}

// Close releases the terminal.
func (l *Live) Close() {
	if l == nil || l.tty == nil {
		return
	}
	l.tty.Close()
}

// oscKeys maps managed colour keys to their OSC numbers.
var oscKeys = map[string]int{
	"foreground":           10,
	"background":           11,
	"cursor-color":         12,
	"selection-background": 17,
	"selection-foreground": 19,
}

// Show pushes a set of colours into the terminal. Missing keys are left as
// they are; call Reset first when switching from a theme that set them.
// Everything goes out in one write so the sequence can never be interleaved
// with a frame the UI is drawing.
func (l *Live) Show(colors map[string]string) {
	if l == nil || l.tty == nil {
		return
	}
	var b strings.Builder
	for key, osc := range oscKeys {
		if v := colors[key]; color.IsHex(v) {
			fmt.Fprintf(&b, "\x1b]%d;%s\x1b\\", osc, color.Normalize(v, v))
		}
	}
	for i := 0; i < 16; i++ {
		if v := colors[fmt.Sprintf("palette.%d", i)]; color.IsHex(v) {
			fmt.Fprintf(&b, "\x1b]4;%d;%s\x1b\\", i, color.Normalize(v, v))
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tty.WriteString(b.String())
}

// Reset returns every colour to what the configuration says, which after a
// reload is the newly applied theme.
func (l *Live) Reset() {
	if l == nil || l.tty == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tty.WriteString("\x1b]104\x1b\\\x1b]110\x1b\\\x1b]111\x1b\\\x1b]112\x1b\\\x1b]117\x1b\\\x1b]119\x1b\\")
}

// ErrReloadUnsupported is returned where no way to ask Ghostty to reload exists.
var ErrReloadUnsupported = errors.New("no way to reload Ghostty from here on this platform")

// Reload asks the running Ghostty to re-read its configuration.
//
// Ghostty has no reload command line and does not watch its files, but it
// has a keybinding for it. On macOS that keystroke can be sent to the app
// through System Events, which needs the user's one-time consent under
// Privacy & Security → Accessibility; when consent is missing the error says
// so and the keystroke is left to the user. On Linux there is nothing to
// call yet, and the hint is the only answer.
func Reload(binary string) error {
	if runtime.GOOS != "darwin" {
		return ErrReloadUnsupported
	}
	keys, mods := reloadKeystroke(binary)
	script := fmt.Sprintf(`tell application "System Events"
	set ghostty to first application process whose bundle identifier is "com.mitchellh.ghostty"
	tell ghostty to keystroke %q using {%s}
end tell`, keys, mods)
	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if strings.Contains(msg, "1002") || strings.Contains(strings.ToLower(msg), "not allowed") || strings.Contains(strings.ToLower(msg), "assistive") {
			return fmt.Errorf("macOS blocked the reload keystroke: allow Ghostty under System Settings → Privacy & Security → Accessibility, or press %s", ReloadHint(binary))
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("could not reload Ghostty: %s", msg)
	}
	return nil
}

// reloadKeystroke reads the reload_config binding from `+list-keybinds` so a
// user who rebound it still gets a working reload. It falls back to the
// default super+shift+,.
func reloadKeystroke(binary string) (key, modifiers string) {
	key, modifiers = ",", "command down, shift down"
	binding, ok := reloadBinding(binary)
	if !ok {
		return
	}
	parts := strings.Split(binding, "+")
	if len(parts) == 0 {
		return
	}
	var mods []string
	for _, p := range parts[:len(parts)-1] {
		switch strings.ToLower(p) {
		case "super", "cmd", "command":
			mods = append(mods, "command down")
		case "shift":
			mods = append(mods, "shift down")
		case "ctrl", "control":
			mods = append(mods, "control down")
		case "alt", "opt", "option":
			mods = append(mods, "option down")
		}
	}
	last := parts[len(parts)-1]
	if len(last) != 1 {
		// Named keys need key codes; keep the default rather than guess.
		return ",", "command down, shift down"
	}
	if len(mods) == 0 {
		return last, ""
	}
	return last, strings.Join(mods, ", ")
}

// reloadBinding returns the trigger bound to reload_config, e.g. "super+shift+,".
func reloadBinding(binary string) (string, bool) {
	if binary == "" {
		return "", false
	}
	output, err := exec.Command(binary, "+list-keybinds").Output()
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasSuffix(line, "=reload_config") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "keybind"))
		value = strings.TrimSpace(strings.TrimPrefix(value, "="))
		trigger := strings.TrimSuffix(value, "=reload_config")
		for _, prefix := range []string{"global:", "unconsumed:", "all:", "performable:"} {
			trigger = strings.TrimPrefix(trigger, prefix)
		}
		if trigger != "" {
			return trigger, true
		}
	}
	return "", false
}

// ReloadHint is the keystroke to tell the user about.
func ReloadHint(binary string) string {
	if binding, ok := reloadBinding(binary); ok {
		return prettyBinding(binding)
	}
	if runtime.GOOS == "darwin" {
		return "⌘⇧,"
	}
	return "Ctrl+Shift+,"
}

func prettyBinding(b string) string {
	if runtime.GOOS != "darwin" {
		return strings.ToUpper(b[:1]) + b[1:]
	}
	r := strings.NewReplacer("super+", "⌘", "cmd+", "⌘", "shift+", "⇧", "ctrl+", "⌃", "alt+", "⌥", "opt+", "⌥")
	return r.Replace(b)
}

// DarkDesktop reports whether the desktop is in dark mode, which decides
// which half of a light:/dark: theme pair is live.
func DarkDesktop() bool {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
		return err == nil && strings.Contains(strings.ToLower(string(out)), "dark")
	default:
		out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
		return err == nil && strings.Contains(string(out), "dark")
	}
}
