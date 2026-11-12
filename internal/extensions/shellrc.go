package extensions

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// The lines that start programs with the shell are kept in one block of the
// shell's start file, between two marker lines, one line per program, each
// ending with a comment naming it. The block is the only part of the file
// this tool writes: what is in it says which programs are on, so nothing is
// remembered elsewhere, and everything outside it is left byte for byte. For
// fish the block is a file of its own in conf.d, which fish reads by itself.

const (
	blockStart = "# >>> ghostty-config >>>"
	blockEnd   = "# <<< ghostty-config <<<"
	blockNote  = "# Programs started by ghostty-config (Extensions → Programs); switch them off there."
	lineMark   = "  # ghostty-config:"
)

// Shells are those whose start file this tool can write.
var Shells = []string{"zsh", "bash", "fish"}

// UserShell is the shell the user works in, by the name of $SHELL, or ""
// when it is none of Shells.
func UserShell() string {
	name := filepath.Base(os.Getenv("SHELL"))
	for _, s := range Shells {
		if name == s {
			return s
		}
	}
	return ""
}

// RcFile is the file the block goes in for a shell. Ghostty starts bash on
// macOS as a login shell, which reads .bash_profile and not .bashrc.
func RcFile(shell, home string) string {
	switch shell {
	case "zsh":
		if dir := os.Getenv("ZDOTDIR"); dir != "" {
			return filepath.Join(dir, ".zshrc")
		}
		return filepath.Join(home, ".zshrc")
	case "bash":
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, ".bash_profile")
		}
		return filepath.Join(home, ".bashrc")
	case "fish":
		config := os.Getenv("XDG_CONFIG_HOME")
		if config == "" {
			config = filepath.Join(home, ".config")
		}
		return filepath.Join(config, "fish", "conf.d", "ghostty-config.fish")
	}
	return ""
}

// UserRcFile is the file of the user's own lines, read to see whether they
// already start a program: for fish config.fish, beside the block's file.
func UserRcFile(shell, home string) string {
	rc := RcFile(shell, home)
	if shell == "fish" {
		return filepath.Join(filepath.Dir(filepath.Dir(rc)), "config.fish")
	}
	return rc
}

// splitBlock cuts a file's text around the block. found is false when the
// file has none, and then before is the whole text.
func splitBlock(text string) (before, block, after string, found bool) {
	start := strings.Index(text, blockStart)
	if start < 0 {
		return text, "", "", false
	}
	end := strings.Index(text[start:], blockEnd)
	if end < 0 {
		return text, "", "", false
	}
	end += start + len(blockEnd)
	if end < len(text) && text[end] == '\n' {
		end++
	}
	return text[:start], text[start:end], text[end:], true
}

// Enabled lists the programs the file's block starts.
func Enabled(text string) []string {
	_, block, _, _ := splitBlock(text)
	var out []string
	for _, l := range strings.Split(block, "\n") {
		if i := strings.LastIndex(l, lineMark); i >= 0 {
			if name := strings.TrimSpace(l[i+len(lineMark):]); name != "PATH" {
				out = append(out, name)
			}
		}
	}
	return out
}

// OwnLine reports whether the text, outside the block, already starts the
// program: a line of the user's own that is not commented out.
func OwnLine(text string, p Program) bool {
	before, _, after, _ := splitBlock(text)
	for _, l := range strings.Split(before+"\n"+after, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if containsAny(l, p.Mark) {
			return true
		}
	}
	return false
}

// pathLine puts ~/.local/bin on the PATH when it is not there already, for
// programs installed from their release.
var pathLine = map[string]string{
	"zsh":  `case ":$PATH:" in *":$HOME/.local/bin:"*) ;; *) export PATH="$HOME/.local/bin:$PATH" ;; esac`,
	"bash": `case ":$PATH:" in *":$HOME/.local/bin:"*) ;; *) export PATH="$HOME/.local/bin:$PATH" ;; esac`,
	"fish": "fish_add_path -g $HOME/.local/bin",
}

// renderBlock writes the block for the programs named, in the order of the
// list; local adds the PATH line first. No program means no block.
func renderBlock(shell string, names []string, local bool) string {
	on := map[string]bool{}
	for _, n := range names {
		on[n] = true
	}
	var lines []string
	for _, p := range Programs {
		if init := p.Init[shell]; on[p.Name] && init != "" {
			lines = append(lines, init+lineMark+p.Name)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	head := []string{blockStart, blockNote}
	if local {
		head = append(head, pathLine[shell]+lineMark+"PATH")
	}
	return strings.Join(append(append(head, lines...), blockEnd), "\n") + "\n"
}

// WithBlock returns the file's text with its block starting exactly the
// programs named. An existing block is replaced where it is. A new one goes
// at the end, but before lines that must stay last: lig's closing line has
// to be the file's last, so the block goes above it and its comments.
func WithBlock(text, shell string, names []string, local bool) string {
	block := renderBlock(shell, names, local)
	if before, _, after, found := splitBlock(text); found {
		if block == "" && strings.HasSuffix(before, "\n\n") {
			before = before[:len(before)-1] // the blank line set above it
		}
		return before + block + after
	}
	if block == "" {
		return text
	}
	lines := strings.SplitAfter(text, "\n")
	at := len(lines)
	for i, l := range lines {
		if containsAny(l, ligMarks) && strings.Contains(l, "post") && !strings.HasPrefix(strings.TrimSpace(l), "#") {
			at = i
			for at > 0 && strings.HasPrefix(strings.TrimSpace(lines[at-1]), "#") {
				at--
			}
			break
		}
	}
	head := strings.Join(lines[:at], "")
	if head != "" && !strings.HasSuffix(head, "\n") {
		head += "\n"
	}
	if head != "" && !strings.HasSuffix(head, "\n\n") {
		head += "\n"
	}
	tail := strings.Join(lines[at:], "")
	if tail != "" {
		block += "\n"
	}
	return head + block + tail
}
