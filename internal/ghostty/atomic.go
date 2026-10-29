package ghostty

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// backedUp remembers which files were copied this run, so a session of many
// small edits leaves one backup per file rather than one per keystroke.
var (
	backedUp   = make(map[string]bool)
	backedUpMu sync.Mutex
)

// backup copies a file's original content into the state directory before its
// first rewrite of the session. fresh asks for a new copy even when one was
// taken already: the file changed under us, and that version is the user's
// too.
func backup(stateDir, path string, original []byte, fresh bool) error {
	backedUpMu.Lock()
	defer backedUpMu.Unlock()
	if backedUp[path] && !fresh {
		return nil
	}
	dir := filepath.Join(stateDir, "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("could not create the backup directory: %w", err)
	}
	stamp := time.Now().Format("20060102-150405")
	if _, err := writeBackup(dir, path, stamp, original); err != nil {
		return fmt.Errorf("could not back up %s: %w", path, err)
	}
	backedUp[path] = true
	return nil
}

// writeBackup copies data, the content of the file at origin, to base.stamp
// in dir, or base.stamp-2, -3 and so on when that name is taken. On macOS the
// XDG and Application Support roots are both called `config`, and a backup
// must never overwrite another one. Which of them a copy was taken of goes in
// the directory's index, since the name cannot say.
func writeBackup(dir, origin, stamp string, data []byte) (string, error) {
	base := filepath.Base(origin)
	for n := 1; ; n++ {
		name := fmt.Sprintf("%s.%s", base, stamp)
		if n > 1 {
			name = fmt.Sprintf("%s-%d", name, n)
		}
		target := filepath.Join(dir, name)
		f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		if abs, err := filepath.Abs(origin); err == nil {
			origin = abs
		}
		return target, recordOrigin(dir, filepath.Base(target), origin)
	}
}

// BackupNow copies every existing root config into the backups directory,
// unconditionally, and returns the paths written.
func BackupNow(paths Paths) ([]string, error) {
	dir := filepath.Join(paths.StateDir, "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	stamp := time.Now().Format("20060102-150405")
	var out []string
	for _, root := range paths.Roots {
		data, err := os.ReadFile(root)
		if err != nil {
			continue
		}
		target, err := writeBackup(dir, root, stamp, data)
		if err != nil {
			return out, err
		}
		out = append(out, target)
	}
	return out, nil
}

// resolveLink follows path to the file it finally names, even when that file
// does not exist yet. A config kept in a dotfiles repository and linked into
// place must stay a link: the rewrite goes to the target, not over the link.
func resolveLink(path string) string {
	for i := 0; i < 40; i++ {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			return path
		}
		target, err := os.Readlink(path)
		if err != nil {
			return path
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		path = target
	}
	return path
}

// writeFileAtomic replaces path in one step: written to a temporary file in
// the same directory, flushed, then renamed over the target. A crash or a
// concurrent reader can never observe a half-written config. A symlink is
// followed and its target replaced; an existing file keeps its permissions,
// and perm applies only to a file created here.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	path = resolveLink(path)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("could not create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("could not create a temporary file: %w", err)
	}
	name := tmp.Name()
	cleanup := func() { os.Remove(name) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(name, path); err != nil {
		cleanup()
		return fmt.Errorf("could not replace %s: %w", path, err)
	}
	return nil
}
