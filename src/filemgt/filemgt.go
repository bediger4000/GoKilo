package filemgt

import (
	"bufio"
	"fmt"
	"io"
	"os"
)

// Open chooses os.Args[1] as a filename to open.
// Returns filename, nil or "", err, or "", nil,
// which is ugly.
func Open(filenames []string, appendF func([]byte)) (string, error) {
	if len(filenames) == 1 {
		return "", nil
	}
	filename := filenames[1]
	fd, er := os.Open(filename)
	if er != nil {
		return "", er
	}
	defer fd.Close()
	fp := bufio.NewReader(fd)

	var err error
	for line, err := fp.ReadBytes('\n'); err == nil; line, err = fp.ReadBytes('\n') {
		// Trim trailing newlines and carriage returns
		for c := line[len(line)-1]; len(line) > 0 && (c == '\n' || c == '\r'); {
			line = line[:len(line)-1]
			if len(line) > 0 {
				c = line[len(line)-1]
			}
		}
		appendF(line)
	}

	if err != nil && err != io.EOF {
		return "", err
	}

	return filename, nil
}

// Save puts all the bytes that func getBytes returns
// into the file named by filename argument. Returns
// a message, and a boolean. The latter indicates whether
// the editor should consider its internal file representation
// as still dirty, or clean.
func Save(filename string, getBytes func() (string, int)) (msg string, stillDirty bool) {
	fp, e := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if e != nil {
		return fmt.Sprintf("Can't save! file open error %s", e), true
	}
	defer fp.Close()
	buf, len := getBytes()
	stillDirty = true
	n, err := io.WriteString(fp, buf)
	if err == nil {
		if n == len {
			stillDirty = false
			msg = fmt.Sprintf("%d bytes written to disk", len)
		} else {
			msg = fmt.Sprintf("wanted to write %d bytes to file, wrote %d", len, n)
		}
		return msg, stillDirty
	}
	return fmt.Sprintf("Can't save! I/O error %s", err), stillDirty
}
