package ghostty

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBinding(t *testing.T) {
	cases := []struct {
		in, value, action string
		prefixes          []string
		steps             int
	}{
		{"super+shift+t=new_tab", "super+shift+t=new_tab", "new_tab", nil, 1},
		{"cmd+shift+T=new_tab", "super+shift+t=new_tab", "new_tab", nil, 1},
		{"shift+opt+control+a=new_tab", "ctrl+alt+shift+a=new_tab", "new_tab", nil, 1},
		{"ctrl+a>n=new_window", "ctrl+a>n=new_window", "new_window", nil, 2},
		{"global:cmd+backquote=toggle_quick_terminal", "global:super+backquote=toggle_quick_terminal", "toggle_quick_terminal", []string{"global"}, 1},
		{"unconsumed:performable:ctrl+c=copy_to_clipboard", "unconsumed:performable:ctrl+c=copy_to_clipboard", "copy_to_clipboard", []string{"unconsumed", "performable"}, 1},
		{"super+==increase_font_size:1", "super+==increase_font_size:1", "increase_font_size:1", nil, 1},
		{"super++=increase_font_size:1", "super++=increase_font_size:1", "increase_font_size:1", nil, 1},
		{"super+digit_3=goto_tab:3", "super+digit_3=goto_tab:3", "goto_tab:3", nil, 1},
		{"super+Digit3=goto_tab:3", "super+digit_3=goto_tab:3", "goto_tab:3", nil, 1},
		{"super+up=jump_to_prompt:-1", "super+arrow_up=jump_to_prompt:-1", "jump_to_prompt:-1", nil, 1},
		{"KeyA=new_tab", "key_a=new_tab", "new_tab", nil, 1},
		{"super+arrow_right=text:\\x05", "super+arrow_right=text:\\x05", "text:\\x05", nil, 1},
		{"resize_split:up,10", "", "", nil, 0},
		{"ctrl+/=new_tab", "ctrl+/=new_tab", "new_tab", nil, 1},
		{"super+t=unbind", "super+t=unbind", "unbind", nil, 1},
		{"f5=reload_config", "f5=reload_config", "reload_config", nil, 1},
	}
	for _, c := range cases {
		b, err := ParseBinding(c.in)
		if c.value == "" {
			if err == nil {
				t.Errorf("%q should not parse, got %+v", c.in, b)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if b.Value() != c.value || b.Action != c.action || len(b.Trigger) != c.steps || strings.Join(b.Prefixes, ",") != strings.Join(c.prefixes, ",") {
			t.Errorf("%q read as %q (%q, %v, %d steps)", c.in, b.Value(), b.Action, b.Prefixes, len(b.Trigger))
		}
	}

	if b, err := ParseBinding("clear"); err != nil || !b.Clear {
		t.Errorf("clear: %+v %v", b, err)
	}
	if b, err := ParseBinding("chain=goto_split:left"); err != nil || !b.Chain || b.Action != "goto_split:left" {
		t.Errorf("chain: %+v %v", b, err)
	}
	if b, err := ParseBinding("vim/ctrl+a=new_window"); err != nil || b.Table != "vim" || b.Trigger.String() != "ctrl+a" {
		t.Errorf("table: %+v %v", b, err)
	}
	for _, bad := range []string{"ctrl+ctrl+a=new_tab", "ctrl+a+b=new_tab", "hyper+a=new_tab", "esc=new_tab", "pgup=new_tab", "global:ctrl+a>n=new_tab", "ctrl+=", "super+shift=new_tab", "ctrl+a>>n=x"} {
		if b, err := ParseBinding(bad); err == nil {
			t.Errorf("%q should be refused, read as %q", bad, b.Value())
		}
	}
}

func TestTriggersPressedTheSameWay(t *testing.T) {
	same := [][2]string{{"super+digit_1", "super+1"}, {"cmd+KeyT", "super+t"}, {"super+comma", "super+,"}, {"ctrl+a>n", "control+a>key_n"}}
	for _, s := range same {
		a, _ := ParseTrigger(s[0])
		b, _ := ParseTrigger(s[1])
		if !a.Same(b) {
			t.Errorf("%s and %s are the same keys", s[0], s[1])
		}
	}
	a, _ := ParseTrigger("super+1")
	b, _ := ParseTrigger("super+shift+1")
	if a.Same(b) {
		t.Error("shift makes a different chord")
	}
}

func TestCatalogFromTheBinaryOutput(t *testing.T) {
	c := BuiltinKeyCatalog(true)
	if len(c.Actions) != 85 {
		t.Fatalf("%d actions read, want 85", len(c.Actions))
	}
	a, ok := c.Action("toggle_quick_terminal")
	if !ok || !strings.HasPrefix(a.Summary, "Toggle the quick terminal.") || strings.Contains(a.Summary, "Quake") {
		t.Fatalf("the summary is the first paragraph: %q", a.Summary)
	}
	if got := c.DefaultsFor("new_tab"); len(got) != 1 || got[0].String() != "super+t" {
		t.Fatalf("new_tab defaults: %v", got)
	}
	if got := c.DefaultsFor("goto_tab:1"); len(got) != 2 {
		t.Fatalf("goto_tab:1 is bound twice by default, physically and by character: %v", got)
	}
	for _, d := range c.Defaults {
		if strings.Contains(d.Action, `\\`) {
			t.Fatalf("backslashes are unescaped: %q", d.Action)
		}
	}
}

// A fake binary answers the two listings; a missing or failing one leaves
// the copy built in.
func TestLoadKeyCatalogRunsTheBinary(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "ghostty")
	script := "#!/bin/sh\ncase \"$1\" in\n+list-actions) printf 'new_tab:\\n  Open a new tab.\\n\\nquit:\\n  Quit Ghostty.\\n';;\n+list-keybinds) echo 'keybind = ctrl+shift+t=new_tab';;\nesac\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	c := LoadKeyCatalog(bin)
	if !c.Live || len(c.Actions) != 2 || c.Actions[0].Summary != "Open a new tab." {
		t.Fatalf("catalog from the binary: %+v", c)
	}
	if len(c.Defaults) != 1 || c.Defaults[0].Value() != "ctrl+shift+t=new_tab" {
		t.Fatalf("defaults from the binary: %+v", c.Defaults)
	}
	if c := LoadKeyCatalog(filepath.Join(dir, "missing")); c.Live || len(c.Actions) != 85 {
		t.Fatalf("a missing binary should fall back to the copy built in, got %d actions", len(c.Actions))
	}
}

func keyTree(t *testing.T, config string) (*Tree, string) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "config")
	if err := os.WriteFile(root, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	tree, err := Load(Paths{Roots: []string{root}, StateDir: filepath.Join(dir, "state")})
	if err != nil {
		t.Fatal(err)
	}
	return tree, root
}

func trig(t *testing.T, s string) Trigger {
	t.Helper()
	tr, err := ParseTrigger(s)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func bind(t *testing.T, s string) Binding {
	t.Helper()
	b, err := ParseBinding(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestKeyMapReplaysTheLinesOverTheDefaults(t *testing.T) {
	defaults := BuiltinKeyCatalog(true).Defaults
	tree, _ := keyTree(t, strings.Join([]string{
		"keybind = super+t=new_window",
		"keybind = super+k=unbind",
		"keybind = ctrl+a>n=new_tab",
		"keybind = chain=goto_split:left",
		"keybind = vim/ctrl+b=new_window",
		"keybind = nonsense",
		"",
	}, "\n"))
	km := tree.KeyMap(defaults)
	if s, ok := km.Lookup(trig(t, "super+t")); !ok || s.Action != "new_window" || !s.User {
		t.Fatalf("super+t should be the user's new_window: %+v", s)
	}
	if _, ok := km.Lookup(trig(t, "super+k")); ok {
		t.Fatal("super+k was unbound")
	}
	if got := km.For("new_tab"); len(got) != 1 || got[0].Trigger.String() != "ctrl+a>n" || len(got[0].Chains) != 1 {
		t.Fatalf("new_tab should keep only the sequence, with its chain: %+v", got)
	}
	if got := km.For("new_window"); len(got) != 2 {
		t.Fatalf("new_window has its default and the user's, not the table's: %+v", got)
	}

	// A clear drops everything before it, defaults included.
	tree, _ = keyTree(t, "keybind = super+t=new_window\nkeybind = clear\nkeybind = ctrl+t=new_tab\n")
	km = tree.KeyMap(defaults)
	if !km.Cleared || len(km.Active) != 1 || km.Active[0].Action != "new_tab" {
		t.Fatalf("after clear only ctrl+t: %+v", km.Active)
	}

	// A chord bound after a sequence that starts with it takes over.
	tree, _ = keyTree(t, "keybind = ctrl+a>n=new_tab\nkeybind = ctrl+a=new_window\n")
	km = tree.KeyMap(nil)
	if len(km.Active) != 1 || km.Active[0].Action != "new_window" {
		t.Fatalf("the sequence should be gone: %+v", km.Active)
	}
	if c := km.Conflicts(trig(t, "ctrl+a>t"), "close_tab"); len(c) != 1 {
		t.Fatalf("a sequence starting with a bound chord conflicts with it: %+v", c)
	}
}

func TestSetShortcutAddsNextToTheUsersBindings(t *testing.T) {
	tree, _ := keyTree(t, "font-size = 13\n# my keys\nkeybind = super+k=clear_screen\nkeybind = chain=scroll_to_top\n\ntheme = x\n")
	tree.SetShortcut(bind(t, "ctrl+shift+n=new_window"), nil)
	want := "font-size = 13\n# my keys\nkeybind = super+k=clear_screen\nkeybind = chain=scroll_to_top\nkeybind = ctrl+shift+n=new_window\n\ntheme = x\n"
	if got := tree.Primary.Text(); got != want {
		t.Fatalf("the new line should follow the user's bindings:\n%s", got)
	}

	// Setting it again rewrites the user's line in place.
	tree.SetShortcut(bind(t, "ctrl+alt+n=new_window"), nil)
	if got := tree.Primary.Text(); !strings.Contains(got, "chain=scroll_to_top\nkeybind = ctrl+alt+n=new_window\n\ntheme") || strings.Contains(got, "ctrl+shift+n") {
		t.Fatalf("the user's own line should be rewritten:\n%s", got)
	}

	// Taking a trigger from another of the user's actions rewrites that line
	// and drops its chain, which belonged to the old action.
	tree.SetShortcut(bind(t, "super+k=new_tab"), nil)
	got := tree.Primary.Text()
	if !strings.Contains(got, "keybind = super+k=new_tab\n#keybind = chain=scroll_to_top  # disabled by ghostty-config") {
		t.Fatalf("super+k should now open a tab, its chain off:\n%s", got)
	}

	// With no binding at all, the line goes to the end of the primary file.
	tree, _ = keyTree(t, "font-size = 13\n")
	tree.SetShortcut(bind(t, "global:super+backquote=toggle_quick_terminal"), nil)
	if got := tree.Primary.Text(); got != "font-size = 13\n\nkeybind = global:super+backquote=toggle_quick_terminal\n" {
		t.Fatalf("appended:\n%q", got)
	}
}

func TestReplacingADefaultSpelledOtherwiseUnbindsIt(t *testing.T) {
	defaults := BuiltinKeyCatalog(true).Defaults
	tree, _ := keyTree(t, "")
	km := tree.KeyMap(defaults)
	tr := trig(t, "super+1")
	if c := km.Conflicts(tr, "new_tab"); len(c) != 2 {
		t.Fatalf("super+1 and super+digit_1 both go to tab 1: %+v", c)
	}
	shadows := km.Shadows(tr, "new_tab")
	if len(shadows) != 1 || shadows[0].String() != "super+digit_1" {
		t.Fatalf("the physical binding outlives the new one: %v", shadows)
	}
	tree.SetShortcut(Binding{Trigger: tr, Action: "new_tab"}, shadows)
	km = tree.KeyMap(defaults)
	if s, _ := km.Lookup(tr); s.Action != "new_tab" {
		t.Fatalf("super+1: %+v", s)
	}
	if got := km.For("goto_tab:1"); len(got) != 0 {
		t.Fatalf("tab 1 lost both its shortcuts: %+v", got)
	}
	if !strings.Contains(tree.Primary.Text(), "keybind = super+1=new_tab\nkeybind = super+digit_1=unbind\n") {
		t.Fatalf("written:\n%s", tree.Primary.Text())
	}

	// Back to the default for tab 1 drops the unbind it is owed.
	tree.ResetShortcuts("goto_tab:1", BuiltinKeyCatalog(true).DefaultsFor("goto_tab:1"))
	if got := tree.KeyMap(defaults).For("goto_tab:1"); len(got) != 1 || got[0].Trigger.String() != "super+digit_1" {
		t.Fatalf("tab 1 should have its physical default back: %+v", got)
	}
}

func TestRemoveAndResetShortcuts(t *testing.T) {
	cat := BuiltinKeyCatalog(true)
	tree, _ := keyTree(t, "keybind = ctrl+t=new_tab\n")
	tree.RemoveShortcut("new_tab", trig(t, "super+t"), cat.DefaultsFor("new_tab"))
	km := tree.KeyMap(cat.Defaults)
	if got := km.For("new_tab"); len(got) != 1 || got[0].Trigger.String() != "ctrl+t" {
		t.Fatalf("only the user's ctrl+t should be left: %+v", got)
	}
	if !strings.Contains(tree.Primary.Text(), "keybind = ctrl+t=new_tab\nkeybind = super+t=unbind\n") {
		t.Fatalf("written:\n%s", tree.Primary.Text())
	}
	tree.RemoveShortcut("new_tab", trig(t, "ctrl+t"), nil)
	if got := tree.KeyMap(cat.Defaults).For("new_tab"); len(got) != 0 {
		t.Fatalf("new_tab should have no shortcut: %+v", got)
	}

	tree.ResetShortcuts("new_tab", cat.DefaultsFor("new_tab"))
	if got := tree.KeyMap(cat.Defaults).For("new_tab"); len(got) != 1 || got[0].User || got[0].Trigger.String() != "super+t" {
		t.Fatalf("back to Ghostty's super+t: %+v", got)
	}
	// Nothing is deleted: the lines are commented out, so the save's guard
	// against lost settings holds.
	if _, err := tree.Save(); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(tree.Primary.Text(), "# disabled by ghostty-config"); n != 2 {
		t.Fatalf("two lines switched off, %d found:\n%s", n, tree.Primary.Text())
	}
}

func TestShortcutPrefix(t *testing.T) {
	tree, _ := keyTree(t, "keybind = unconsumed:super+backquote=toggle_quick_terminal\n")
	tree.SetShortcutPrefix("toggle_quick_terminal", "global", true)
	if got := tree.Primary.Text(); got != "keybind = global:unconsumed:super+backquote=toggle_quick_terminal\n" {
		t.Fatalf("global on: %q", got)
	}
	tree.SetShortcutPrefix("toggle_quick_terminal", "global", false)
	if got := tree.Primary.Text(); got != "keybind = unconsumed:super+backquote=toggle_quick_terminal\n" {
		t.Fatalf("global off: %q", got)
	}
}

// The edits are replayed by value when the file changed on disk before the
// save: lines moved, and line numbers would land on the wrong ones.
func TestShortcutEditsSurviveAnOutsideEdit(t *testing.T) {
	cat := BuiltinKeyCatalog(true)
	tree, root := keyTree(t, "keybind = super+k=clear_screen\nkeybind = ctrl+t=new_tab\n")
	tree.SetShortcut(bind(t, "ctrl+shift+n=new_window"), nil)
	tree.RemoveShortcut("new_tab", trig(t, "ctrl+t"), nil)
	tree.SetShortcutPrefix("new_window", "global", true)

	// Someone adds lines above, and a binding of their own, meanwhile.
	os.WriteFile(root, []byte("# added\nfont-size = 15\nkeybind = super+k=clear_screen\nkeybind = ctrl+t=new_tab\nkeybind = super+j=scroll_to_bottom\n"), 0o644)
	if _, err := tree.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(root)
	got := string(data)
	for _, want := range []string{"# added\nfont-size = 15\n", "keybind = super+j=scroll_to_bottom\nkeybind = global:ctrl+shift+n=new_window\n", "#keybind = ctrl+t=new_tab  # disabled by ghostty-config"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q after the replay:\n%s", want, got)
		}
	}
	km := tree.KeyMap(cat.Defaults)
	if s, _ := km.Lookup(trig(t, "super+j")); s.Action != "scroll_to_bottom" {
		t.Fatalf("the outside binding is kept: %+v", s)
	}
}
