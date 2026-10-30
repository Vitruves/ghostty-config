package tui

import "sort"

// Each section of the menu is split into a few named parts, so a long list
// reads as groups: the picture interface titles them and lists them beside
// the settings, and the menu keeps the commands of a part together.

var subsections = map[string][][2]string{
	"Themes": {
		{"Browse", "theme favs fav"},
		{"Create", "new random edit revert save fork"},
		{"Files", "delete"},
	},
	"Fonts": {
		{"Family", "fixfont font style size"},
		{"Text", "lineheight cellwidth ligatures thicken thickness faint"},
		// The font's own features and axes are named at run time.
		{"Features", ""},
		{"Variable axes", ""},
		{"Icons", "icons"},
	},
	"Window": {
		{"Presets", "preset"},
		{"Title bar", "titlebar shadow decoration gtktitlebar"},
		{"Appearance", "opacity blur windowtheme colorspace contrast opaquecells"},
		{"Padding", "paddingx paddingy balance paddingcolor"},
		{"Splits", "splitopacity draghandle"},
		{"Background image", "bgimage bgopacity bgfit bgposition bgrepeat"},
		{"Size at launch", "columns rows windowx windowy"},
	},
	"Input": {
		{"Keyboard", "keys optionalt"},
		{"Cursor", "cursor blink cursoropacity clicktomove"},
		{"Clipboard", "copy clearselect trimspaces pasteguard clipread clipwrite middleclick"},
		{"Mouse", "hidemouse focusmouse scrollbar osc8"},
	},
	"Session": {
		{"Windows", "savestate inheritcwd stepresize quitlast tabbar tabsinverted tabswipe confirmclose resizeoverlay"},
		{"Quick terminal", "quickposition quicksize quickscreen quickanimation quickhide quickspaces quickkeyboard quicklayer"},
		{"Scrollback", "scrollback scrollbacklines compression"},
		{"Programs", "exittime vtresize cliplimit"},
		{"Notifications", "notify"},
		{"Bell", "bellsound bellaudio bellfile bellvolume bellbounce belltitle bellborder"},
		{"Shell integration", "shell shellcursor shelltitle shellsudo shellpath sshenv sshterminfo sshhosts"},
	},
	"Extensions": {
		{"Shaders", "shader"},
		{"Theme packs", "pack"},
	},
	"Tool": {
		{"Ghostty", "reload autoreload"},
		{"Interface", "interface"},
		{"History", "undo changes"},
		{"Config", "check restore overrides collection backup paths"},
		{"Help", "help quit"},
	},
}

// subOf is the part of its section a command belongs to, and the rank of
// that part; commands no part names fall in one named after the section.
func subOf(c *command) (string, int) {
	for i, part := range subsections[c.group] {
		if c.set != nil && c.set.part == part[0] {
			return part[0], i
		}
		for _, name := range splitWords(part[1]) {
			if name == c.name {
				return part[0], i
			}
		}
	}
	return c.group, len(subsections[c.group])
}

func splitWords(s string) []string {
	var out []string
	start := -1
	for i, r := range s + " " {
		if r == ' ' {
			if start >= 0 {
				out = append(out, s[start:i])
			}
			start = -1
		} else if start < 0 {
			start = i
		}
	}
	return out
}

// orderBySub keeps the commands of each part together, parts in their order.
func orderBySub(opts []option) {
	sort.SliceStable(opts, func(a, b int) bool {
		if opts[a].cmd == nil || opts[b].cmd == nil {
			return false
		}
		_, ra := subOf(opts[a].cmd)
		_, rb := subOf(opts[b].cmd)
		return ra < rb
	})
}
