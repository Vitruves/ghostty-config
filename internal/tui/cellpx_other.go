//go:build !unix

package tui

func cellPixels() (w, h float64) { return 0, 0 }
