package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/vitruves/ghostty-config/internal/ghostty"
)

// The hosts Ghostty 1.4 has installed its terminfo on over ssh, read from
// `ghostty +ssh-cache`. Forgetting one only means the next connection to it
// installs the terminfo again, so a single host goes at once; pruning and
// clearing, which take many, ask first.

// sshPruneAge is the age the prune row forgets hosts beyond, in Ghostty's
// duration syntax.
const sshPruneAge = "30d"

// sshHostsFresh is how long a read of the cache serves the list: it is
// rebuilt on every keystroke, and the cache only changes when ssh runs.
const sshHostsFresh = 3 * time.Second

// sshHostsCommand lists the cache and forgets what is chosen. Only for a
// Ghostty that has +ssh-cache in the 1.4 form.
func sshHostsCommand() *command {
	return &command{name: "sshhosts", group: "Session", label: "SSH hosts", syntax: "sshhosts",
		desc:    "The hosts ssh-terminfo has installed Ghostty's terminfo on, from ghostty +ssh-cache; a host forgotten gets it again on its next connection",
		options: func(m *Model, arg string) []option { return m.sshHostsOptions(arg) },
		run: func(m *Model, arg string, opt *option) tea.Cmd {
			if opt == nil || opt.value == "" {
				return nil
			}
			switch {
			case opt.value == "prune":
				m.openConfirm(fmt.Sprintf("Forget every host the terminfo was installed on more than %s ago? Each gets it again on its next connection.", ageWord(sshPruneAge)), func(m *Model) tea.Cmd {
					n, err := ghostty.SSHCachePrune(m.paths.Binary, sshPruneAge)
					if err != nil {
						m.fail("Could not prune the SSH cache: %v", err)
					} else {
						m.info("Forgot %s older than %s", countWord(n, "host"), ageWord(sshPruneAge))
					}
					m.sshHostsAt = time.Time{}
					m.recompute()
					return nil
				})
			case opt.value == "clear":
				m.openConfirm(fmt.Sprintf("Forget all %s in the SSH cache? Each gets Ghostty's terminfo again on its next connection.", countWord(len(m.sshHosts), "host")), func(m *Model) tea.Cmd {
					if err := ghostty.SSHCacheClear(m.paths.Binary); err != nil {
						m.fail("Could not clear the SSH cache: %v", err)
					} else {
						m.info("Forgot every host: the terminfo is installed again on each next connection")
					}
					m.sshHostsAt = time.Time{}
					m.recompute()
					return nil
				})
			case strings.HasPrefix(opt.value, "host:"):
				dest := strings.TrimPrefix(opt.value, "host:")
				if err := ghostty.SSHCacheRemove(m.paths.Binary, dest); err != nil {
					m.fail("Could not forget %s: %v", dest, err)
				} else {
					m.info("Forgot %s: its next connection installs the terminfo again", dest)
				}
				m.sshHostsAt = time.Time{}
			}
			return nil
		},
		preview: previewSSHHostsPanel,
	}
}

// sshHostsOptions are the cached hosts, then what forgets more than one.
func (m *Model) sshHostsOptions(arg string) []option {
	if m.paths.Binary == "" {
		return []option{{label: "Ghostty's program was not found", detail: "there is no cache to read", header: true}}
	}
	if time.Since(m.sshHostsAt) > sshHostsFresh {
		m.sshHosts, m.sshHostsErr = ghostty.SSHCache(m.paths.Binary)
		m.sshHostsAt = time.Now()
	}
	if m.sshHostsErr != nil {
		return []option{{label: "Could not read the SSH cache", detail: m.sshHostsErr.Error(), header: true}}
	}
	if len(m.sshHosts) == 0 {
		return []option{{label: "No host in the SSH cache", detail: "ssh-terminfo adds each host it installs the terminfo on", header: true}}
	}
	needle := strings.TrimSpace(arg)
	var out []option
	for _, e := range m.sshHosts {
		if needle != "" && !fuzzyMatch(e.Dest, needle) {
			continue
		}
		out = append(out, option{label: e.Dest, detail: e.Age, value: "host:" + e.Dest})
	}
	actions := []option{
		{label: "Forget hosts older than " + ageWord(sshPruneAge), value: "prune"},
		{label: "Forget every host", detail: countWord(len(m.sshHosts), "host"), value: "clear"},
	}
	hosts := len(out)
	for _, o := range actions {
		if needle != "" && !fuzzyMatch(o.label, needle) {
			continue
		}
		if len(out) == hosts && hosts > 0 {
			// A blank line between the hosts and what forgets many.
			out = append(out, option{header: true})
		}
		out = append(out, o)
	}
	return out
}

// ageWord says a duration of Ghostty's, 30d, in words.
func ageWord(age string) string {
	if n, ok := strings.CutSuffix(age, "d"); ok {
		return n + " days"
	}
	return age
}

func previewSSHHostsPanel(m *Model, opt *option, width int) []string {
	c := m.c
	lines := []string{c.bold(c.fg).Render(truncate("SSH hosts", width))}
	switch {
	case opt == nil || opt.value == "":
		return append(lines, para(c.mutedS(), m.cmd.desc+".", width)...)
	case opt.value == "prune":
		return append(lines, para(c.base(), "Forgets every host the terminfo was installed on more than "+ageWord(sshPruneAge)+" ago, after asking: ghostty +ssh-cache --prune="+sshPruneAge+".", width)...)
	case opt.value == "clear":
		return append(lines, para(c.base(), "Empties the cache, after asking: ghostty +ssh-cache --clear. Every host gets Ghostty's terminfo again on its next connection.", width)...)
	}
	dest := strings.TrimPrefix(opt.value, "host:")
	lines = []string{c.bold(c.fg).Render(truncate(dest, width))}
	for _, e := range m.sshHosts {
		if e.Dest == dest && !e.Added.IsZero() {
			lines = append(lines, c.mutedS().Render(truncate("installed "+e.Added.Local().Format("2 January 2006, 15:04")+"  ·  "+e.Age, width)))
		}
	}
	lines = append(lines, "")
	return append(lines, para(c.base(), "Enter forgets this host: ghostty +ssh-cache --remove="+dest+". Its next connection through ssh-terminfo installs the terminfo again. A host on another port is a destination of its own.", width)...)
}
