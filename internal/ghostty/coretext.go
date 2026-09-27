package ghostty

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// On macOS Ghostty resolves fonts through Core Text. Asking the same database
// removes the guesswork: the style names come back spelled exactly as Ghostty
// will match them against `font-style`, and the monospace trait is the real
// one rather than a guess from the name.
const coreTextScript = `
ObjC.import('AppKit');
var fm = $.NSFontManager.sharedFontManager;
var fams = fm.availableFontFamilies;
var out = [];
for (var i = 0; i < fams.count; i++) {
  var fam = ObjC.unwrap(fams.objectAtIndex(i));
  if (fam.charAt(0) === '.') continue;
  var members = fm.availableMembersOfFontFamily($(fam));
  if (!members) continue;
  var styles = [], mono = false;
  for (var j = 0; j < members.count; j++) {
    var m = members.objectAtIndex(j);
    var style = ObjC.unwrap(m.objectAtIndex(1));
    var traits = ObjC.unwrap(m.objectAtIndex(3));
    if ((traits & 1024) !== 0) mono = true;
    styles.push(style);
  }
  out.push(fam + '\t' + (mono ? '1' : '0') + '\t' + styles.join('|'));
}
out.join('\n');
`

// coreTextFamilies returns every family with its styles and monospace flag.
// NSFontMonoSpaceTrait is bit 10 of the trait mask, tested in the script.
func coreTextFamilies() (map[string]platformFamily, error) {
	cmd := exec.Command("osascript", "-l", "JavaScript", "-e", coreTextScript)
	cmd.Env = append(os.Environ(), "LC_ALL=en_US.UTF-8")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("could not read the Core Text font list: %w", err)
	}
	out := make(map[string]platformFamily)
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.Split(strings.TrimSpace(line), "\t")
		if len(parts) < 3 || parts[0] == "" {
			continue
		}
		var styles []string
		for _, s := range strings.Split(parts[2], "|") {
			if s = strings.TrimSpace(s); s != "" {
				styles = append(styles, s)
			}
		}
		out[parts[0]] = platformFamily{Styles: styles, Mono: parts[1] == "1"}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("Core Text reported no families")
	}
	return out, nil
}
