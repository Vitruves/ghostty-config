//go:build unix

package tui

import (
	"os"

	"golang.org/x/sys/unix"
)

// cellPixels is the size in device pixels of one terminal cell, from the
// pixel size the terminal reports for its window; zero when it reports none.
func cellPixels() (w, h float64) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 || ws.Row == 0 || ws.Xpixel == 0 || ws.Ypixel == 0 {
		return 0, 0
	}
	return float64(ws.Xpixel) / float64(ws.Col), float64(ws.Ypixel) / float64(ws.Row)
}
