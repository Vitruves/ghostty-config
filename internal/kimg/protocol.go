package kimg

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/x/ansi/kitty"
)

// The pictures reach the terminal as Unicode placeholders: the image is sent
// once and given a virtual placement, and the cells where it should appear
// hold the placeholder character with two combining marks naming the row and
// the column of the picture each cell shows. The cell's foreground colour is
// the image's number. Because the pictures are ordinary text cells, they
// move, scroll and disappear with the rest of the screen, and the library
// that draws the screen needs to know nothing about them.

const chunk = 4096

// ID colours: an image number is carried as a 24-bit colour.
func IDColour(id uint32) (r, g, b uint8) {
	return uint8(id >> 16), uint8(id >> 8), uint8(id)
}

// Transmit is the escape sequence that sends a PNG and creates its virtual
// placement, cols by rows cells large.
func Transmit(id uint32, data []byte, cols, rows int) string {
	enc := base64.StdEncoding.EncodeToString(data)
	var b strings.Builder
	first := true
	for len(enc) > 0 {
		n := chunk
		if n > len(enc) {
			n = len(enc)
		}
		part := enc[:n]
		enc = enc[n:]
		more := 0
		if len(enc) > 0 {
			more = 1
		}
		if first {
			fmt.Fprintf(&b, "\x1b_Ga=T,f=100,U=1,i=%d,c=%d,r=%d,q=2,m=%d;%s\x1b\\", id, cols, rows, more, part)
			first = false
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d;%s\x1b\\", more, part)
		}
	}
	return b.String()
}

// DeleteAll removes every picture and frees its memory.
func DeleteAll() string { return "\x1b_Ga=d,d=A,q=2\x1b\\" }

// Delete removes one picture and frees its memory.
func Delete(id uint32) string { return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id) }

// diacritics is how many rows or columns the combining marks can name.
const diacritics = 297

// PlaceholderRow is the text of one row of a picture: cols placeholder
// cells. It carries no colour; the caller sets the foreground to IDColour.
//
// The marks name only the first 297 columns. Past them a cell carries the
// row alone, and the terminal takes the column from the cell to its left,
// one further on: the protocol infers it for a placeholder whose left
// neighbour shows the same picture on the same row, which every cell of a
// row is. The columns that have a mark keep it, so a row cut from the left,
// as under a dialog, still shows the right part of the picture.
func PlaceholderRow(row, cols int) string {
	var b strings.Builder
	for c := 0; c < cols; c++ {
		b.WriteRune(kitty.Placeholder)
		b.WriteRune(kitty.Diacritic(row))
		if c < diacritics {
			b.WriteRune(kitty.Diacritic(c))
		}
	}
	return b.String()
}

// TransmitOnly sends a PNG without showing it: the picture waits under its
// number until a placement names it.
func TransmitOnly(id uint32, data []byte) string {
	enc := base64.StdEncoding.EncodeToString(data)
	var b strings.Builder
	first := true
	for len(enc) > 0 {
		n := chunk
		if n > len(enc) {
			n = len(enc)
		}
		part := enc[:n]
		enc = enc[n:]
		more := 0
		if len(enc) > 0 {
			more = 1
		}
		if first {
			fmt.Fprintf(&b, "\x1b_Ga=t,f=100,i=%d,q=2,m=%d;%s\x1b\\", id, more, part)
			first = false
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d;%s\x1b\\", more, part)
		}
	}
	return b.String()
}

// Place shows a transmitted picture at a cell, cols by rows cells large,
// below the text but above the cells' background colours (z = -1). The
// cursor is saved and restored round the move, so the program that draws the
// screen never notices.
func Place(id, placement uint32, col, row, cols, rows int) string {
	return fmt.Sprintf("\x1b7\x1b[%d;%dH\x1b_Ga=p,i=%d,p=%d,c=%d,r=%d,z=-1,C=1,q=2\x1b\\\x1b8", row+1, col+1, id, placement, cols, rows)
}

// Unplace removes one placement and leaves the picture in memory.
func Unplace(id, placement uint32) string {
	return fmt.Sprintf("\x1b_Ga=d,d=i,i=%d,p=%d,q=2\x1b\\", id, placement)
}

// TransmitRGBA sends raw 8-bit RGBA pixels, w by h, and creates a virtual
// placement cols by rows cells large, like Transmit. With a file path the
// terminal reads the pixels from that temporary file and deletes it, which
// costs nothing on the pty; without one they go inline, compressed.
func TransmitRGBA(id uint32, pix []byte, w, h, cols, rows int, file string) string {
	enc := ""
	if file == "" {
		enc = EncodeRGBA(pix)
	}
	return TransmitEncoded(id, enc, w, h, cols, rows, file)
}

// EncodeRGBA compresses pixels the way TransmitRGBA sends them inline. It
// is the slow part of sending, and needs no number, so pictures can be
// compressed side by side and sent in order after.
func EncodeRGBA(pix []byte) string {
	z := compressors.Get().(*compressor)
	z.out.Reset()
	z.zw.Reset(&z.out)
	_, _ = z.zw.Write(pix)
	_ = z.zw.Close()
	enc := base64.StdEncoding.EncodeToString(z.out.Bytes())
	compressors.Put(z)
	return enc
}

// compressor is a zlib writer and the buffer it writes to, kept between
// pictures: the writer holds the best part of a megabyte of tables, a frame
// compresses dozens of pictures, and the buffer would otherwise grow anew
// for each of them.
type compressor struct {
	zw  *zlib.Writer
	out bytes.Buffer
}

var compressors = sync.Pool{New: func() any {
	zw, _ := zlib.NewWriterLevel(nil, zlib.BestSpeed)
	return &compressor{zw: zw}
}}

// TransmitEncoded is TransmitRGBA for pixels EncodeRGBA compressed, or,
// with a file path, for pixels written to that file.
func TransmitEncoded(id uint32, enc string, w, h, cols, rows int, file string) string {
	var b strings.Builder
	WriteTransmitEncoded(&b, id, enc, w, h, cols, rows, file)
	return b.String()
}

// WriteTransmitEncoded is TransmitEncoded written to b, which grows once
// to take it.
func WriteTransmitEncoded(b *strings.Builder, id uint32, enc string, w, h, cols, rows int, file string) {
	ctrl := fmt.Sprintf("a=T,f=32,s=%d,v=%d,U=1,i=%d,c=%d,r=%d,q=2", w, h, id, cols, rows)
	if file != "" {
		b.WriteString("\x1b_G" + ctrl + ",t=t;" + base64.StdEncoding.EncodeToString([]byte(file)) + "\x1b\\")
		return
	}
	// Each chunk is wrapped in at most "\x1b_Gm=1;" and "\x1b\\", the
	// first in the control data as well.
	b.Grow(len(enc) + (len(enc)/chunk+1)*len("\x1b_Gm=1;\x1b\\") + len(ctrl) + len(",o=z,"))
	first := true
	for len(enc) > 0 {
		n := chunk
		if n > len(enc) {
			n = len(enc)
		}
		part := enc[:n]
		enc = enc[n:]
		more := "0"
		if len(enc) > 0 {
			more = "1"
		}
		if first {
			b.WriteString("\x1b_G")
			b.WriteString(ctrl)
			b.WriteString(",o=z,m=")
			first = false
		} else {
			b.WriteString("\x1b_Gm=")
		}
		b.WriteString(more)
		b.WriteString(";")
		b.WriteString(part)
		b.WriteString("\x1b\\")
	}
}
