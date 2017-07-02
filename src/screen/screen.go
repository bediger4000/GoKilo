package screen

import (
	"fmt"
	"log"
	"io"
	"os"
	"syscall"
	"unsafe"
)

type winSize struct {
	row    uint16
	col    uint16
	xpixel uint16
	ypixel uint16
}

func getCursorPosition() (rows int, cols int, worked bool) {
	io.WriteString(os.Stdout, "\x1b[6n")
	var buffer [1]byte
	var buf []byte
	var cc int
	for cc, _ = os.Stdin.Read(buffer[:]); cc == 1; cc, _ = os.Stdin.Read(buffer[:]) {
		if buffer[0] == 'R' {
			break
		}
		buf = append(buf, buffer[0])
	}
	if string(buf[0:2]) != "\x1b[" {
		log.Printf("Failed to read rows;cols from tty\n")
		return -1, -1, false
	}
	if n, e := fmt.Sscanf(string(buf[2:]), "%d;%d", &rows, &cols); n != 2 || e != nil {
		if e != nil {
			log.Printf("GetCursorPosition: fmt.Sscanf() failed: %s\n", e)
		}
		if n != 2 {
			log.Printf("getCursorPosition: got %d items, wanted 2\n", n)
		}
		return -1, -1, false
	}
	return rows, cols, true
}

func GetWindowSize() (rows int, cols int, worked bool) {
	var w winSize
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL,
		os.Stdout.Fd(),
		syscall.TIOCGWINSZ,
		uintptr(unsafe.Pointer(&w)),
	)
	if err == 0 { // type syscall.Errno
		return int(w.row), int(w.col), true
	}
	io.WriteString(os.Stdout, "\x1b[999C\x1b[999B")
	return getCursorPosition()
}
