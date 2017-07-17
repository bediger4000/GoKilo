package main

import (
	"editor"
	"filemgt"
	"fmt"
	"io"
	"os"
	"tty"
)

func main() {

	var err error

	E, err := editor.NewEditor()
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(1)
	}

	E.Filename, err = filemgt.Open(os.Args, E.AppendRow)
	if err != nil {
		fmt.Printf("%s\n", err)
	}

	ttyDev := new(tty.Tty)
	ttyDev.EnableRawMode()
	defer ttyDev.DisableRawMode()
	E.Dirty = false
	E.UpdateAllSyntax()

	E.SetStatusMessage("HELP: Ctrl-S = save | Ctrl-Q = quit | Ctrl-F = find")

	for {
		E.RefreshScreen()
		again, e := E.ProcessKeypress()
		if !again {
			break
		}
		if e != nil {
			fmt.Printf("%s\n", e)
			break
		}
	}
	io.WriteString(os.Stdout, "\x1b[2J")
	io.WriteString(os.Stdout, "\x1b[H")
}
