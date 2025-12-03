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
	"github.com/xyproto/files"
	"github.com/xyproto/vt"
)

type State struct {
	c             *vt.Canvas
	dir1          string
	dir2          string
	dir3          string
	quit          bool
	startx        uint
	starty        uint
	prompt_length uint
	written       []rune
	bashMode      bool
}

const (
	start_message  = "---=[ MegaCLI ]=---"
	ctrl_c_message = "bye (ctrl-c pressed)"
	exit_message   = "bye"

	leftArrow  = "←"
	rightArrow = "→"
	upArrow    = "↑"
	downArrow  = "↓"

	pgUpKey = "⇞" // page up
	pgDnKey = "⇟" // page down
	homeKey = "⇱" // home
	endKey  = "⇲" // end
)

func ulen[T string | []rune | []int | []uint](xs T) uint {
	return uint(len(xs))
}

func (s *State) drawOutput(text string) {
	lines := strings.Split(text, "\n")
	x := s.startx
	y := s.starty + 1
	for _, line := range lines {
		vt.SetXY(x, y)
		s.c.Write(x, y, vt.Default, vt.BackgroundDefault, strings.TrimSpace(line))
		y++
	}
}

func (s *State) drawError(text string) {
	lines := strings.Split(text, "\n")
	x := s.startx
	y := s.starty + 1
	for _, line := range lines {
		vt.SetXY(x, y)
		s.c.Write(x, y, vt.Red, vt.BackgroundDefault, line)
		y++
	}
}

func (s *State) ls(dir string) {
	const margin = 1
	longestSoFar := uint(0)
	entries, err := os.ReadDir(dir)
	if err == nil { // success
		x := s.startx
		y := s.starty + 1
		//vt.SetXY(x, y)
		for _, e := range entries {
			name := e.Name()
			if ulen(name) > longestSoFar {
				longestSoFar = ulen(name)
			}
			path := filepath.Join(dir, name)
			if isdir(path) {
				s.c.Write(x, y, vt.Blue, vt.BackgroundDefault, name)
				s.c.Write(x+ulen(name), y, vt.White, vt.BackgroundDefault, "/")
			} else if isexec(path) {
				s.c.Write(x, y, vt.LightGreen, vt.BackgroundDefault, name)
				s.c.Write(x+ulen(name), y, vt.White, vt.BackgroundDefault, "*")
			} else if files.IsSymlink(path) {
				s.c.Write(x, y, vt.LightRed, vt.BackgroundDefault, name)
				s.c.Write(x+ulen(name), y, vt.White, vt.BackgroundDefault, ">")
			} else if files.IsBinary(path) {
				s.c.Write(x, y, vt.LightMagenta, vt.BackgroundDefault, name)
				s.c.Write(x+ulen(name), y, vt.White, vt.BackgroundDefault, "b")
			} else {
				s.c.Write(x, y, vt.Default, vt.BackgroundDefault, name)
			}
			y++
			if y >= s.c.H() {
				x += longestSoFar + margin
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

func isexec(path string) bool {
	return files.IsExecutableCached(path)
}

func (s *State) edit(filename, path string) error {
	editorPath, err := exec.LookPath(env.Str("EDITOR"))
	if err != nil {
		return err
	}
	command := exec.Command(editorPath, filename)
	command.Dir = path
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	return command.Run()
}

func run(executableName, path string) error {
	executablePath, err := exec.LookPath(executableName)
	if err != nil {
		return err
	}
	command := exec.Command(executablePath)
	command.Dir = path
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	return command.Run()
}

func shellRun(shellCommand, path string) (string, error) {
	command := exec.Command("bash", "-c", shellCommand)
	command.Dir = path
	command.Env = env.Environ()
	outBytes, err := command.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(outBytes), nil
}

func (s *State) execute(cmd, path string) (changedDirectory bool) {
	// Common for non-bash and bash mode
	if cmd == "exit" || cmd == "quit" || cmd == "q" {
		s.quit = true
	} else if isdir(filepath.Join(path, cmd)) { // relative path
		newPath := filepath.Join(path, cmd)
		if s.dir1 != newPath {
			s.dir1 = newPath
			changedDirectory = true
		}
	} else if isdir(cmd) { // absolute path
		if s.dir1 != cmd {
			s.dir1 = cmd
			changedDirectory = true
		}
	} else if isfile(filepath.Join(path, cmd)) { // relative path
		s.edit(cmd, path)
	} else if isfile(cmd) { // abs absolute path
		s.edit(cmd, path)
	} else {
		if s.bashMode { // only for bash mode
			if output, err := shellRun(cmd, s.dir1); err != nil {
				s.drawError(err.Error())
			} else {
				s.drawOutput(output)
			}
		} else { // only for non-bash mode
			if cmd == "l" || cmd == "ls" || strings.HasPrefix(cmd, "l ") {
				rest := ""
				if len(cmd) > 2 {
					rest = cmd[2:]
				}
				if rest != "" {
					s.ls(rest)
				} else {
					s.ls(path)
				}
			} else if foundExecutableInPath := files.WhichCached(cmd); foundExecutableInPath != "" {
				run(foundExecutableInPath, s.dir1)
			} else {
				s.drawError("?")
			}
		}
	}
	return
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
		x, y uint
		s    = &State{
			c:        c,
			dir1:     ".",
			dir2:     env.HomeDir(),
			dir3:     "/tmp",
			quit:     false,
			startx:   uint(5),
			starty:   uint(6),
			bashMode: false,
		}
	)

	draw_prompt := func() {
		prompt := "$ "
		if !s.bashMode {
			if absPath, err := filepath.Abs(s.dir1); err == nil { // success
				prompt = absPath + "> "
			} else {
				prompt = s.dir1 + "> "
			}
		}
		c.Write(s.startx, s.starty, vt.LightGreen, vt.BackgroundDefault, prompt)
		s.prompt_length = ulen(prompt)
	}

	// The rune index for the text that has been written
	index := uint(0)

	draw_written := func() {
		x = s.startx + s.prompt_length
		y = s.starty
		c.Write(x, y, vt.LightYellow, vt.BackgroundDefault, string(s.written))
		r := rune(' ')
		if index < ulen(s.written) {
			r = s.written[index]
		}
		c.WriteRune(x+index, y, vt.Black, vt.BackgroundYellow, r)
		vt.SetXY(x, y)

	}

	clear_written := func() {
		y := s.starty
		for x := s.startx + s.prompt_length; x < c.W(); x++ {
			c.WriteRune(x, y, vt.LightYellow, vt.BackgroundDefault, ' ')
		}
		vt.SetXY(x, y)
	}

	clear_and_prepare := func() {
		c.Clear()
		c.Write(5, 5, vt.LightGreen, vt.BackgroundDefault, start_message)
		draw_prompt()
		x = s.startx + s.prompt_length
		y = s.starty
		draw_written()
	}

	clear_and_prepare()
	c.Draw()

	for !s.quit {
		key := tty.String()
		switch key {
		case "c:27", "c:17": // esc, ctrl-q
			s.quit = true
		case "c:13": // return
			if len(s.written) == 0 {
				s.written = []rune("ls")
			}
			clear_and_prepare()
			s.execute(string(s.written), s.dir1)
			clear_written()
			s.written = []rune{}
			index = 0
			draw_written()
		case "c:127": // backspace
			clear_written()
			if len(s.written) > 0 {
				s.written = append(s.written[:index-1], s.written[index:]...)
				index--
			}
			draw_written()
		case "c:11": // ctrl-k
			clear_written()
			if len(s.written) > 0 {
				s.written = s.written[:index]
			}
			draw_written()
		case "c:4": // ctrl-d
			clear_written()
			if len(s.written) > 0 {
				s.written = append(s.written[:index], s.written[index+1:]...)
			}
			draw_written()
		case "c:1", homeKey, upArrow: // ctrl-a, home, arrow up
			clear_written()
			index = 0
			draw_written()
		case "c:5", endKey, downArrow: // ctrl-e, end, arrow down
			clear_written()
			index = ulen(s.written) // one after the text
			draw_written()
		case leftArrow:
			clear_written()
			if index > 0 {
				index--
			}
			draw_written()
		case rightArrow:
			clear_written()
			if index < ulen(s.written) {
				index++
			}
			draw_written()
		case "c:9": // tab
			if len(s.written) == 0 {
				s.bashMode = !s.bashMode
				clear_written()
				clear_and_prepare()
				draw_written()
				break
			}
			clear_written()
			last_word_written_so_far := string(s.written)
			if fields := strings.Fields(last_word_written_so_far); len(fields) > 1 {
				last_word_written_so_far = fields[len(fields)-1]
			}
			if entries, err := os.ReadDir(s.dir1); err == nil { // success
				for _, entry := range entries {
					name := entry.Name()
					if strings.HasPrefix(name, last_word_written_so_far) {
						rest := []rune(name)[len(last_word_written_so_far):]
						s.written = append(s.written, rest...)
						index += ulen(rest)
						break
					}
				}
			}
		OUT:
			for _, p := range env.Path() {
				if entries, err := os.ReadDir(p); err == nil { // success
					for _, entry := range entries {
						name := entry.Name()
						if strings.HasPrefix(name, last_word_written_so_far) && files.IsExecutable(filepath.Join(p, name)) {
							rest := []rune(name)[len(s.written):]
							s.written = append(s.written, rest...)
							index += ulen(rest)
							break OUT
						}
					}
				}
			}
			draw_written()
		case "c:12": // ctrl-l
			c.Clear()
		case "c:0": // ctrl-space
			run("tig", s.dir1)
		case "c:3": // ctrl-c
			cleanupFunc()
			fmt.Fprintln(os.Stderr, ctrl_c_message)
			os.Exit(1)
		case "":
			continue
		default:
			clear_written()
			s.written = append(s.written, []rune(key)...)
			index++
			draw_written()
		}
		c.Draw()
	}

	c.Write(10, 10, vt.LightRed, vt.BackgroundDefault, exit_message)
}
