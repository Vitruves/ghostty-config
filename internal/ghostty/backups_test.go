package ghostty

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRestoreBackupRoundTrip(t *testing.T) {
	p, dir := fixture(t, map[string]string{"dotfiles/ghostty": "font-size = 10\n"})
	real := filepath.Join(dir, "dotfiles", "ghostty")
	os.Chmod(real, 0o600)
	link := p.Roots[0]
	os.MkdirAll(filepath.Dir(link), 0o755)
	if err := os.Symlink("../dotfiles/ghostty", link); err != nil {
		t.Fatal(err)
	}
	tree, _ := Load(p)
	tree.Set("font-size", "12")
	save(t, tree) // backs up font-size = 10 first

	backups, err := ListBackups(p)
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v, %v", backups, err)
	}
	b := backups[0]
	if target, guessed := tree.BackupTarget(b); target != link || guessed {
		t.Fatalf("target = %s (guessed %v), want %s", target, guessed, link)
	}
	saved, err := RestoreBackup(p, b, link)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, real); got != "font-size = 10\n" {
		t.Fatalf("restored %q", got)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the restore replaced the link")
	}
	if info, _ := os.Stat(real); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
	// What was there before the restore is a backup of its own, the newest.
	if got := read(t, saved); got != "font-size = 12\n" {
		t.Fatalf("the replaced file was saved as %q", got)
	}
	backups, _ = ListBackups(p)
	if len(backups) != 2 || backups[0].Path != saved {
		t.Fatalf("after the restore: %+v", backups)
	}
}

func TestBackupsListNewestFirstAndOldOnesFindTheirFile(t *testing.T) {
	p, dir := fixture(t, map[string]string{
		"xdg/config":     "font-size = 10\ntheme = Dracula\n",
		"support/config": "window-padding-x = 4\n",
	})
	backupDir := filepath.Join(dir, "state", "backups")
	os.MkdirAll(backupDir, 0o755)
	// Copies from before the index existed: only their names.
	os.WriteFile(filepath.Join(backupDir, "config.20250101-090000"), []byte("window-padding-x = 2\n"), 0o644)
	os.WriteFile(filepath.Join(backupDir, "config.20250101-090000-2"), []byte("font-size = 9\ntheme = Dracula\n"), 0o644)
	os.WriteFile(filepath.Join(backupDir, "notes.txt"), []byte("not a backup"), 0o644)
	backups, err := ListBackups(p)
	if err != nil || len(backups) != 2 {
		t.Fatalf("backups = %+v, %v", backups, err)
	}
	if backups[0].Seq != 2 || !backups[0].Taken.Equal(time.Date(2025, 1, 1, 9, 0, 0, 0, time.Local)) {
		t.Fatalf("newest first: %+v", backups)
	}
	tree, _ := Load(p)
	if target, guessed := tree.BackupTarget(backups[0]); target != p.Roots[0] || !guessed {
		t.Fatalf("the font backup goes to %s (guessed %v)", target, guessed)
	}
	if target, _ := tree.BackupTarget(backups[1]); target != p.Roots[2] {
		t.Fatalf("the padding backup goes to %s", target)
	}
}
