// package main is the main package for the MegaFile program
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xyproto/env/v2"
	"github.com/xyproto/files"
	"github.com/xyproto/megafile"
	"github.com/xyproto/vt"
)

const (
	versionString = "MegaFile 1.3.13"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version":
			fmt.Println(versionString)
			return
		case "-h", "--help":
			fmt.Print(usageString)
			return
		}
	}

	// Initialize vt terminal settings
	vt.Init()

	// Prepare a canvas
	c := vt.NewCanvas()
	defer megafile.Cleanup(c)

	// Handle ctrl-c
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		megafile.Cleanup(c)
		os.Exit(1)
	}()

	tty, err := vt.NewTTY()
	if err != nil {
		megafile.Cleanup(c)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer tty.Close()
	tty.SetTimeout(10 * time.Millisecond)

	startdirs := []string{".", env.HomeDir(), "/tmp"}
	if len(os.Args) > 1 && files.IsDir(os.Args[1]) {
		// Use command-line argument as the first directory, if it is a directory
		startdirs = []string{os.Args[1], env.HomeDir(), "/tmp"}
	}
	curdir, err := megafile.New(c, tty, startdirs, "", env.StrAlt("EDITOR", "vi")).Run()
	if err != nil && err != megafile.ErrExit {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Write the current directory path to stderr at exit, so that shell scripts can use it
	fmt.Fprintln(os.Stderr, curdir)
}
