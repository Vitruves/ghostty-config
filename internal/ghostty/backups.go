package ghostty

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Backups are copies of config files in the state directory, named after the
// file and the moment they were taken: config.20261020-141503, with -2, -3
// for copies taken in the same second. originsFile, in the state directory
// rather than among the copies, says which file each one is a copy of.

const originsFile = "backup-origins"

var originsMu sync.Mutex

// recordOrigin adds a line "name<TAB>path" to the index of the backups
// directory dir.
func recordOrigin(dir, name, origin string) error {
	originsMu.Lock()
	defer originsMu.Unlock()
	f, err := os.OpenFile(filepath.Join(filepath.Dir(dir), originsFile), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, "%s\t%s\n", name, origin); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// readOrigins reads the index; a backup taken before there was one is
// simply not in it.
func readOrigins(dir string) map[string]string {
	originsMu.Lock()
	defer originsMu.Unlock()
	out := map[string]string{}
	f, err := os.Open(filepath.Join(filepath.Dir(dir), originsFile))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if name, origin, ok := strings.Cut(sc.Text(), "\t"); ok {
			out[name] = origin
		}
	}
	return out
}

// Backup is one copy in the backups directory.
type Backup struct {
	// Path is the copy itself.
	Path string
	// Base is the name of the file it was taken of, and Origin that file's
	// path when the index knows it.
	Base, Origin string
	// Taken is when, to the second; Seq tells apart copies of the same second.
	Taken time.Time
	Seq   int
}

var backupName = regexp.MustCompile(`^(.+)\.(\d{8}-\d{6})(?:-(\d+))?$`)

// ListBackups lists the backups, newest first.
func ListBackups(paths Paths) ([]Backup, error) {
	dir := filepath.Join(paths.StateDir, "backups")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	origins := readOrigins(dir)
	var out []Backup
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := backupName.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		taken, err := time.ParseInLocation("20060102-150405", m[2], time.Local)
		if err != nil {
			continue
		}
		seq := 1
		if m[3] != "" {
			seq, _ = strconv.Atoi(m[3])
		}
		out = append(out, Backup{Path: filepath.Join(dir, e.Name()), Base: m[1], Origin: origins[e.Name()], Taken: taken, Seq: seq})
	}
	sort.SliceStable(out, func(a, b int) bool {
		if !out[a].Taken.Equal(out[b].Taken) {
			return out[a].Taken.After(out[b].Taken)
		}
		return out[a].Seq > out[b].Seq
	})
	return out, nil
}

// BackupTarget is the file a backup would be restored over. The index says
// so for every copy taken since it exists. An older copy has only its base
// name, which on macOS two roots share: it goes to the loaded file of that
// name whose text is closest to it, and guessed says so.
func (t *Tree) BackupTarget(b Backup) (path string, guessed bool) {
	if b.Origin != "" {
		return b.Origin, false
	}
	var candidates []string
	for _, d := range t.Docs {
		if filepath.Base(d.Path) == b.Base {
			candidates = append(candidates, d.Path)
		}
	}
	for _, root := range t.Paths.Roots {
		if filepath.Base(root) == b.Base && t.doc(root) == nil {
			candidates = append(candidates, root)
		}
	}
	switch len(candidates) {
	case 0:
		return "", true
	case 1:
		return candidates[0], false
	}
	data, err := os.ReadFile(b.Path)
	if err != nil {
		return candidates[0], true
	}
	copyLines := SplitLines(string(data))
	best, bestCost := candidates[0], -1
	for _, c := range candidates {
		now, _ := os.ReadFile(c)
		added, removed := DiffCounts(Diff(SplitLines(string(now)), copyLines))
		if cost := added + removed; bestCost < 0 || cost < bestCost {
			best, bestCost = c, cost
		}
	}
	return best, true
}

// SplitLines cuts text into lines, without the newline that ends the last.
func SplitLines(text string) []string {
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// RestoreBackup puts a backup back over target. The file as it is goes into
// the backups first, so the restore can itself be taken back, and the write
// is atomic, follows a link to the real file and keeps its permissions. It
// returns the path of the copy of the file it replaced, empty when there was
// no file.
func RestoreBackup(paths Paths, b Backup, target string) (string, error) {
	data, err := os.ReadFile(b.Path)
	if err != nil {
		return "", fmt.Errorf("could not read the backup: %w", err)
	}
	saved := ""
	if current, err := os.ReadFile(target); err == nil {
		dir := filepath.Join(paths.StateDir, "backups")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		saved, err = writeBackup(dir, target, time.Now().Format("20060102-150405"), current)
		if err != nil {
			return "", fmt.Errorf("could not back up %s first: %w", target, err)
		}
	}
	if err := writeFileAtomic(target, data, 0o644); err != nil {
		return saved, err
	}
	return saved, nil
}
