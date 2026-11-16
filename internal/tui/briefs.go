package tui

// briefs says in a few words what a command that is not a setting opens or
// does. The menu writes it under the command's name: a line with only a
// name and an arrow says too little about what Enter would do.
var briefs = map[string]string{
	"font":   "Pick a family, or install one",
	"style":  "The face used for regular text",
	"size":   "In points, half points allowed",
	"preset": "A bundle of window, font and input settings",
	"keys":   "Every shortcut; press a new one to change it",

	"shader":  "Cursor trails and screen effects",
	"pack":    "Themes that do not ship with Ghostty",
	"program": "Command-line tools that put Ghostty to use",

	"reload":     "Have Ghostty read its files again now",
	"undo":       "Take back the last change, one step at a time",
	"changes":    "What this session changed in each file",
	"check":      "Ask Ghostty which lines it rejects",
	"restore":    "Bring a file back from a backup",
	"overrides":  "Comment out the lines that shadow every theme",
	"collection": "Install or refresh the curated themes",
	"backup":     "Copy every config file to the backups now",
	"paths":      "Where the config, themes and backups live",
	"help":       "The keys, and how your files are treated",
	"quit":       "Leave; nothing is applied on the way out",
}

// actionH is the height of a menu line that carries its brief.
const actionH = 54

// briefOf is the line under a command of the menu, "" for a setting.
func briefOf(c *command) string {
	if c == nil || c.set != nil {
		return ""
	}
	return briefs[c.name]
}
