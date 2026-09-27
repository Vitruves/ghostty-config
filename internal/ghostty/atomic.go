package ghostty

import (
	"fmt"
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
// first rewrite of the session.
func backup(stateDir, path string, original []byte) error {
	backedUpMu.Lock()
	defer backedUpMu.Unlock()
	if backedUp[path] {
		return nil
	}
	dir := filepath.Join(stateDir, "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("could not create the backup directory: %w", err)
	}
	name := fmt.Sprintf("%s.%s", filepath.Base(path), time.Now().Format("20060102-150405"))
	if err := os.WriteFile(filepath.Join(dir, name), original, 0o644); err != nil {
		return fmt.Errorf("could not back up %s: %w", path, err)
	}
	backedUp[path] = true
	return nil
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
		target := filepath.Join(dir, fmt.Sprintf("%s.%s", filepath.Base(root), stamp))
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return out, err
		}
		out = append(out, target)
	}
	return out, nil
}

// writeFileAtomic replaces path in one step: written to a temporary file in
// the same directory, flushed, then renamed over the target. A crash or a
// concurrent reader can never observe a half-written config.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
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
