package ghostty

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
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

// LiveOn paints through a file that is already open, such as a terminal
// reached another way, or a plain file in a test that reads back what was
// sent. Close closes it.
func LiveOn(f *os.File) *Live {
	if f == nil {
		return nil
	}
	return &Live{tty: f}
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

// ErrNotFrontmost is returned when Ghostty was not the app in front, so the
// reload keystroke was not sent: it would have gone to whichever app was.
// Only the keystroke, kept for Ghostty before 1.3, can meet it.
var ErrNotFrontmost = errors.New("Ghostty was not in front, so it was not asked to reload")

// What the scripts answer when they did nothing.
const (
	notFrontmost = "ghostty-config:not-frontmost"
	noTerminal   = "ghostty-config:no-terminal"
)

// Reload asks the running Ghostty to re-read its configuration.
//
// Ghostty has no reload command line and does not watch its files, but its
// apps take the reload_config action from outside. On macOS, Ghostty 1.3 and
// later perform it over AppleScript, which macOS allows once the user agrees
// under Privacy & Security → Automation. Older versions, or one with
// macos-applescript off, get the reload keystroke through System Events
// instead, which needs Accessibility and Ghostty in front. On Linux the GTK
// app takes it over D-Bus.
func Reload(binary string) error {
	if runtime.GOOS == "darwin" {
		return reloadMac(binary)
	}
	return reloadGTK(binary)
}

func reloadMac(binary string) error {
	id := macBundleID()
	output, err := exec.Command("osascript", "-e", performScript(id)).CombinedOutput()
	msg := strings.TrimSpace(string(output))
	if err == nil && msg == "true" {
		return nil
	}
	// -1743 is macOS refusing this terminal the right to script Ghostty. The
	// keystroke needs another permission, which the user may have given.
	denied := err != nil && strings.Contains(msg, "-1743")
	kerr := sendReloadKeystroke(binary, id)
	if kerr != nil && denied {
		return fmt.Errorf("macOS blocked the reload: allow your terminal to control Ghostty under System Settings → Privacy & Security → Automation, or press %s", ReloadHint(binary))
	}
	return kerr
}

// macBundleID is the Ghostty to talk to. macOS hands every process an app
// starts its bundle identifier, so a debug build reloads itself; anything
// else gets the release app.
func macBundleID() string {
	if id := os.Getenv("__CFBundleIdentifier"); strings.HasPrefix(id, "com.mitchellh.ghostty") {
		return id
	}
	return "com.mitchellh.ghostty"
}

// performScript is the AppleScript that has Ghostty perform reload_config.
// The action belongs to the app, so any terminal will do. It never launches
// a Ghostty that is not running, and an older Ghostty, whose dictionary has
// no perform action, fails to compile it.
func performScript(id string) string {
	return fmt.Sprintf(`if application id %[1]q is not running then return %[2]q
tell application id %[1]q
	if (count of terminals) is 0 then return %[2]q
	return perform action "reload_config" on terminal 1
end tell`, id, noTerminal)
}

// sendReloadKeystroke types the reload binding into Ghostty through System
// Events, the way to reach Ghostty before 1.3.
func sendReloadKeystroke(binary, id string) error {
	keys, mods := reloadKeystroke(binary)
	output, err := exec.Command("osascript", "-e", reloadScript(id, keys, mods)).CombinedOutput()
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
	if strings.TrimSpace(string(output)) == notFrontmost {
		return ErrNotFrontmost
	}
	return nil
}

// reloadScript is the AppleScript that sends the reload keystroke. System
// Events types into whichever app is in front, whatever process the tell
// names, so the keystroke is only sent while that app is Ghostty; otherwise
// the script answers notFrontmost and sends nothing.
func reloadScript(id, keys, mods string) string {
	return fmt.Sprintf(`tell application "System Events"
	set ghostty to first application process whose bundle identifier is %q
	if frontmost of ghostty is false then return %q
	tell ghostty to keystroke %q using {%s}
end tell`, id, notFrontmost, keys, mods)
}

// reloadGTK activates the GTK app's reload-config action over D-Bus, as
// `ghostty +new-window` reaches it. The bus name is the app id, the class
// setting when there is one, and only names someone owns are called: D-Bus
// would otherwise start a new Ghostty to answer.
func reloadGTK(binary string) error {
	gdbus, err := exec.LookPath("gdbus")
	if err != nil {
		return ErrReloadUnsupported
	}
	reloaded, failure := false, ""
	for _, id := range gtkAppIDs(binary) {
		owned, _ := exec.Command(gdbus, "call", "--session", "--dest", "org.freedesktop.DBus",
			"--object-path", "/org/freedesktop/DBus", "--method", "org.freedesktop.DBus.NameHasOwner", id).Output()
		if !strings.Contains(string(owned), "true") {
			continue
		}
		output, err := exec.Command(gdbus, "call", "--session", "--dest", id, "--object-path", gtkObjectPath(id),
			"--method", "org.gtk.Actions.Activate", "reload-config", "[]", "{}").CombinedOutput()
		if err != nil {
			if failure = strings.TrimSpace(string(output)); failure == "" {
				failure = err.Error()
			}
			continue
		}
		reloaded = true
	}
	switch {
	case reloaded:
		return nil
	case failure != "":
		return fmt.Errorf("could not reload Ghostty: %s", failure)
	}
	// No Ghostty on the bus: gtk-single-instance is off, or none runs.
	return ErrReloadUnsupported
}

// gtkAppIDs are the ids a running Ghostty may hold: the class setting, then
// the release id and the one debug builds use.
func gtkAppIDs(binary string) []string {
	var ids []string
	if binary != "" {
		if out, err := exec.Command(binary, "+show-config").Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if class, ok := strings.CutPrefix(strings.TrimSpace(line), "class = "); ok && strings.Contains(class, ".") {
					ids = append(ids, strings.TrimSpace(class))
				}
			}
		}
	}
	for _, id := range []string{"com.mitchellh.ghostty", "com.mitchellh.ghostty-debug"} {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids
}

// gtkObjectPath is where GApplication exports an app id's actions.
func gtkObjectPath(id string) string {
	return "/" + strings.NewReplacer(".", "/", "-", "_").Replace(id)
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
// which half of a light:/dark: theme pair is live. The first call after
// Prefetch takes the answer it started asking for; the others ask again.
func DarkDesktop() bool {
	darkAhead.Lock()
	ahead := darkAhead.answer
	darkAhead.answer = nil
	darkAhead.Unlock()
	if ahead != nil {
		return <-ahead
	}
	return askDarkDesktop()
}

// darkAhead is the answer Prefetch started asking for, until taken.
var darkAhead struct {
	sync.Mutex
	answer chan bool
}

func prefetchDarkDesktop() {
	answer := make(chan bool, 1)
	darkAhead.Lock()
	darkAhead.answer = answer
	darkAhead.Unlock()
	go func() { answer <- askDarkDesktop() }()
}

func askDarkDesktop() bool {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
		return err == nil && strings.Contains(strings.ToLower(string(out)), "dark")
	default:
		out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
		return err == nil && strings.Contains(string(out), "dark")
	}
}
