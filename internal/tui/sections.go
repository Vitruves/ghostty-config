package tui

import (
	"sort"
	"sync"
)

// Each section of the menu is split into a few named parts, so a long list
// reads as groups: the picture interface titles them and lists them beside
// the settings, and the menu keeps the commands of a part together.
//
// The parts of a section come in the order people reach for them, and the
// settings of a part in the order its words are written here: what shows
// first, then what tunes it.
var subsections = map[string][][2]string{
	"Themes": {
		{"Browse", "theme favs fav"},
		{"Create", "new random edit revert save fork"},
		{"Files", "delete"},
	},
	"Fonts": {
		{"Family", "fixfont font style size"},
		{"Text", "ligatures shapingbreak lineheight cellwidth thicken thickness faint"},
		// The font's own features and axes are named at run time.
		{"Features", ""},
		{"Variable axes", ""},
		{"Bold and italic", "boldfont italicfont bolditalicfont boldstyle italicstyle bolditalicstyle fakebold fakeitalic fakebolditalic boldcolor"},
		{"Icons", "icons iconheight"},
		{"Metrics", "baseline underlinepos underlinethick strikepos strikethick overlinepos overlinethick boxthick"},
	},
	"Window": {
		{"Presets", "preset"},
		{"Appearance", "opacity blur opaquecells windowtheme colorspace contrast shaderanimation"},
		{"Title bar", "titlebar shadow decoration gtktitlebar titlestyle"},
		{"Padding", "paddingx paddingy balance paddingcolor"},
		{"Size at launch", "columns rows windowx windowy maximize"},
		{"Title", "title subtitle titlefont windowbuttons proxyicon titlebarbg titlebarfg hidetitlemax toolbarstyle"},
		{"Full screen", "fullscreen fullscreenstyle"},
		{"Tabs", "newtab tabbar tabslocation widetabs tabsinverted tabswipe"},
		{"Splits", "splitopacity splitfill dividercolor draghandle splitzoom"},
		{"Colours", "palettegen paletteharmonious searchbg searchfg searchselbg searchselfg"},
		{"Background image", "bgimage bgopacity bgfit bgposition bgrepeat"},
	},
	"Input": {
		{"Keyboard", "optionalt keys"},
		{"Cursor", "cursor blink cursoropacity cursorthick cursorheight"},
		{"Mouse", "hidemouse focusmouse clicktomove scrollbar rightclick middleclick scrollwheel scrolltrackpad clickinterval mousereporting shiftcapture"},
		{"Selection", "copy clearselect clearoncopy wordchars"},
		{"Clipboard", "trimspaces pasteguard bracketedsafe clipread clipwrite"},
		{"Links", "osc8 linkpreviews"},
	},
	"Session": {
		{"Windows", "savestate inheritcwd tabinheritcwd splitinheritcwd inheritfontsize confirmclose undotimeout quitlast quitdelay stepresize resizeoverlay resizeposition resizeduration"},
		{"Startup", "command initialcommand workdir initialwindow waitafter"},
		{"Scrollback", "scrollback scrollbacklines compression scrollonkey scrollonoutput"},
		{"Notifications", "notify notifyafter notifybell notifydesktop desktopnotify toastcopy toastreload"},
		{"Bell", "bellsound bellaudio bellfile bellvolume bellbounce belltitle bellborder"},
		{"Quick terminal", "quickposition quicksize quickscreen quickanimation quickhide quickspaces quickkeyboard quicklayer quickns"},
		{"Shell integration", "shell shellcursor shelltitle shellsudo shellpath sshenv sshterminfo sshhosts"},
		{"Programs", "exittime vtresize cliplimit progress imagelimit"},
		{"Updates", "autoupdate updatechannel"},
		{"macOS", "icon iconframe iconghost iconscreen customicon dockdrop machidden secureinput secureindicator shortcuts applescript"},
		{"Linux", "singleinstance class x11name language cgroup cgroupfail cgroupmemory cgroupprocs"},
		{"Advanced", "term enquiry oscreport titlereport kam checksum checksumnegate checksumattributes checksumtrim checksumundrawn checksumfull graphemewidth alphablending vsync asyncbackend hinting autohint forceautohint lighthinting monochrome opengldebug"},
	},
	"Extensions": {
		{"Shaders", "shader"},
		{"Theme packs", "pack"},
		{"Programs", "program"},
	},
	"Tool": {
		{"Ghostty", "reload autoreload"},
		{"Interface", "layout interface accent uicontrast uisize cards"},
		{"History", "undo changes"},
		{"Config", "check restore overrides collection backup paths"},
		{"Help", "help quit"},
	},
}

// subPlace is where a command sits: the part of its section, that part's
// rank, and its own rank among the commands of the section.
type subPlace struct {
	part       string
	rank, slot int
}

// subIndex is subsections read once, by section and command word: the menu
// asks where a command sits for every line of every frame.
var subIndex = sync.OnceValue(func() map[string]map[string]subPlace {
	idx := map[string]map[string]subPlace{}
	for group, parts := range subsections {
		idx[group] = map[string]subPlace{}
		for i, part := range parts {
			for j, name := range splitWords(part[1]) {
				idx[group][name] = subPlace{part[0], i, i*subSlots + j}
			}
		}
	}
	return idx
})

// subSlots is more settings than a part ever holds; the ones made at run
// time, which no list names, come after the named ones in the order made.
const subSlots = 1000

func placeOf(c *command) subPlace {
	if c.set != nil && c.set.part != "" {
		for i, part := range subsections[c.group] {
			if part[0] == c.set.part {
				return subPlace{part[0], i, i*subSlots + subSlots - 1}
			}
		}
	}
	if p, ok := subIndex()[c.group][c.name]; ok {
		return p
	}
	n := len(subsections[c.group])
	return subPlace{c.group, n, n * subSlots}
}

// subOf is the part of its section a command belongs to, and the rank of
// that part; commands no part names fall in one named after the section.
func subOf(c *command) (string, int) {
	p := placeOf(c)
	return p.part, p.rank
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

// orderBySub keeps the commands of each part together, parts in their order
// and each part's commands in the order it lists them.
func orderBySub(opts []option) {
	sort.SliceStable(opts, func(a, b int) bool {
		if opts[a].cmd == nil || opts[b].cmd == nil {
			return false
		}
		return placeOf(opts[a].cmd).slot < placeOf(opts[b].cmd).slot
	})
}
