package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/xyproto/env/v2"
	"github.com/xyproto/vt"
)

type State struct {
	dir1          string
	c             *vt.Canvas
	quit          bool
	startx        uint
	starty        uint
	prompt_length uint
	written       string
}

const (
	start_message  = "---=[ MegaCLI ]=---"
	ctrl_c_message = "bye (ctrl-c pressed)"
	exit_message   = "bye"
)

func ulen(s string) uint {
	return uint(len(s))
}

func (s *State) ls(dir string) {
	longestSoFar := uint(0)
	entries, err := os.ReadDir(dir)
	if err == nil { // success
		x := s.startx
		y := s.starty + 1
		vt.SetXY(x, y)
		for _, e := range entries {
			if ulen(e.Name()) > longestSoFar {
				longestSoFar = ulen(e.Name())
			}
			s.c.WriteString(x, y, vt.LightBlue, vt.BackgroundDefault, e.Name())
			y++
			if y >= s.c.H() {
				x += longestSoFar
				y = s.starty
			}
		}
	} else {
		fmt.Fprintln(os.Stderr, "Could not list "+dir)
	}
}

func isdir(path string) bool {
	if fi, err := os.Stat(path); err == nil { // success
		return fi.Mode().IsDir()
	}
	return false
}

func isfile(path string) bool {
	if fi, err := os.Stat(path); err == nil { // success
		return fi.Mode().IsRegular()
	}
	return false
}

func (s *State) open(path string) error {
	editorPath, err := exec.LookPath(env.Str("EDITOR"))
	if err != nil {
		return err
	}
	command := exec.Command(editorPath, path)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func run(executableName string) error {
	executablePath, err := exec.LookPath(executableName)
	if err != nil {
		return err
	}
	command := exec.Command(executablePath)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func (s *State) execute(cmd string) {
	if cmd == "l" || cmd == "ls" || strings.HasPrefix(cmd, "l ") {
		rest := ""
		if len(cmd) > 2 {
			rest = cmd[2:]
		}
		if rest != "" {
			s.ls(rest)
		} else {
			s.ls(s.dir1)
		}
	} else if cmd == "exit" || cmd == "quit" || cmd == "q" {
		s.quit = true
	} else if isdir(filepath.Join(s.dir1, cmd)) { // relative path
		s.dir1 = filepath.Join(s.dir1, cmd)
	} else if isdir(cmd) { // absolute path
		s.dir1 = cmd
	} else if isfile(filepath.Join(s.dir1, cmd)) { // relative path
		s.open(filepath.Join(s.dir1, cmd))
	} else if isfile(cmd) { // abs absolute path
		s.open(cmd)
	}
}

func main() {
	// Initialize vt terminal settings
	vt.Init()

	// Prepare a canvas
	c := vt.NewCanvas()
	cleanupFunc := func() {
		vt.SetXY(0, c.H()-1)
		c.Clear()
		vt.SetLineWrap(true)
		vt.ShowCursor(true)
		//vt.Home() // also clears the screen?
	}
	defer cleanupFunc()

	// Handle ctrl-c
	ch := make(chan os.Signal)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		cleanupFunc()
		fmt.Fprintln(os.Stderr, ctrl_c_message)
		os.Exit(1)
	}()

	tty, err := vt.NewTTY()
	if err != nil {
		cleanupFunc()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer tty.Close()
	tty.SetTimeout(10 * time.Millisecond)

	var (
		s    = &State{dir1: ".", c: c, quit: false, startx: uint(7), starty: uint(6)}
		x, y uint
	)

	draw_prompt := func() {
		prompt := s.dir1 + "> "
		c.WriteString(s.startx, s.starty, vt.LightGreen, vt.BackgroundDefault, prompt)
		s.prompt_length = ulen(prompt)
	}

	clear_and_prepare := func() {
		c.Clear()
		c.Write(5, 5, vt.LightGreen, vt.BackgroundDefault, start_message)
		draw_prompt()
		x = s.startx + s.prompt_length
		y = s.starty
		c.WriteRune(x, y, vt.LightMagenta, vt.BackgroundDefault, '_')
		vt.SetXY(x, y)
	}

	draw_written := func() {
		x = s.startx + s.prompt_length
		y = s.starty
		c.WriteString(x, y, vt.LightYellow, vt.BackgroundDefault, s.written)
		vt.SetXY(x, y)

	}

	clear_written := func() {
		y := s.starty
		for x := s.startx + s.prompt_length; x < c.W(); x++ {
			c.WriteRune(x, y, vt.LightYellow, vt.BackgroundDefault, ' ')
		}
		vt.SetXY(x, y)
	}

	clear_and_prepare()
	c.Draw()

	index := uint(0)

	for !s.quit {
		key := tty.String()
		switch key {
		case "c:27", "c:17": // esc, ctrl-q
			s.quit = true
		case "c:13": // return
			if s.written == "" {
				s.written = "ls"
			}
			clear_and_prepare()
			tmpdir := s.dir1
			s.execute(s.written)
			if tmpdir != s.dir1 {
				clear_and_prepare()
			}
			s.written = ""
		case "c:127": // backspace
			clear_written()
			if index > 0 {
				index--
			}
			if len(s.written) > 0 {
				s.written = s.written[:len(s.written)-1]
			}
			draw_written()
		case "c:4": // ctrl-d
			clear_written()
			if len(s.written) > 0 {
				s.written = s.written[:index] + s.written[index+1:]
			}
			draw_written()
		case "c:9": // tab
			clear_written()
			if entries, err := os.ReadDir(s.dir1); err == nil { // success
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(), s.written) {
						rest := entry.Name()[len(s.written):]
						s.written += rest
						break
					}
				}
			}
			draw_written()
		case "c:12": // ctrl-l
			c.Clear()
		case "c:0": // ctrl-space
			run("tig")
		case "c:3": // ctrl-c
			cleanupFunc()
			fmt.Fprintln(os.Stderr, ctrl_c_message)
			os.Exit(1)
		case "":
			continue
		default:
			clear_written()
			x++
			s.written += key
			draw_written()
		}
		c.Draw()
	}

	c.WriteString(10, 10, vt.LightRed, vt.BackgroundDefault, exit_message)
}
