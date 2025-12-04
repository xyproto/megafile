// package main is the main package for the MegaCLI program
package main

import (
	"errors"
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

// State holds the current state of the shell, then canvas and the directory structures
type State struct {
	c            *vt.Canvas
	dir          []string
	dirIndex     uint
	quit         bool
	startx       uint
	starty       uint
	promptLength uint
	written      []rune
	prevdir      []string
	showHidden   bool
}

const (
	versionString = "MegaCLI 1.0.3"

	startMessage = "---=[ MegaCLI ]=---"

	leftArrow  = "←"
	rightArrow = "→"
	upArrow    = "↑"
	downArrow  = "↓"

	pgUpKey = "⇞" // page up
	pgDnKey = "⇟" // page down
	homeKey = "⇱" // home
	endKey  = "⇲" // end

	bashDollarColor = vt.LightRed
	angleColor      = vt.LightRed
	promptColor     = vt.LightGreen
	headerColor     = vt.LightMagenta
)

func ulen[T string | []rune | []string](xs T) uint {
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

func (s *State) ls(dir string) error {
	const margin = 1
	var (
		x            = s.startx
		y            = s.starty + 1
		w            = s.c.W()
		longestSoFar = uint(0)
	)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if !s.showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if ulen(name) > longestSoFar {
			longestSoFar = ulen(name)
		}
		path := filepath.Join(dir, name)
		if files.IsDir(path) {
			s.c.Write(x, y, vt.Blue, vt.BackgroundDefault, name)
			s.c.Write(x+ulen(name), y, vt.White, vt.BackgroundDefault, "/")
		} else if files.IsExecutableCached(path) {
			s.c.Write(x, y, vt.LightGreen, vt.BackgroundDefault, name)
			s.c.Write(x+ulen(name), y, vt.White, vt.BackgroundDefault, "*")
		} else if files.IsSymlink(path) {
			s.c.Write(x, y, vt.LightRed, vt.BackgroundDefault, name)
			s.c.Write(x+ulen(name), y, vt.White, vt.BackgroundDefault, "^")
		} else if files.IsBinary(path) {
			s.c.Write(x, y, vt.LightMagenta, vt.BackgroundDefault, name)
			s.c.Write(x+ulen(name), y, vt.White, vt.BackgroundDefault, "¤")
		} else {
			s.c.Write(x, y, vt.Default, vt.BackgroundDefault, name)
		}
		y++
		if y >= s.c.H() {
			x += longestSoFar + margin
			y = s.starty + 1
		}
		if x+longestSoFar > w {
			break
		}
	}
	return nil
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

func run(executableName string, args []string, path string) error {
	executablePath, err := exec.LookPath(executableName)
	if err != nil {
		return err
	}
	command := exec.Command(executablePath, args...)
	command.Dir = path
	command.Env = env.Environ()
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	return command.Run()
}

func run2(executableName string, args []string, path string) (string, error) {
	command := exec.Command(executableName, args...)
	command.Dir = path
	command.Env = env.Environ()
	outBytes, err := command.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(outBytes), nil
}

// shellRun works okay, but have issues when running ie. "htop"
func shellRun(shellCommand, path string) (string, error) {
	shellExecutable := files.WhichCached(env.Str("SHELL"))
	if shellExecutable == "" {
		shellExecutable = "bash"
	}
	args := []string{"-c", shellCommand}
	switch filepath.Base(shellExecutable) {
	case "bash", "zsh":
		args = []string{"-i", "-c", shellCommand + ";exit"}
	}
	command := exec.Command(shellExecutable, args...)
	command.Dir = path
	command.Env = env.Environ()
	command.Stdin = os.Stdin
	outBytes, err := command.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(outBytes), nil
}

func (s *State) setPath(path string) {
	absPath, err := filepath.Abs(path)
	if err == nil { // success
		s.prevdir[s.dirIndex] = s.dir[s.dirIndex]
		s.dir[s.dirIndex] = absPath
	} else {
		s.prevdir[s.dirIndex] = s.dir[s.dirIndex]
		s.dir[s.dirIndex] = path
	}
}

// execute tries to execute the given command in the given directory,
// and returns true if the directory was changed
// and returns true if a file was edited
// and returns an error if something went wrong
func (s *State) execute(cmd, path string) (bool, bool, error) {
	// Common for non-bash and bash mode
	if cmd == "exit" || cmd == "quit" || cmd == "q" || cmd == "bye" {
		s.quit = true
		return false, false, nil
	}
	if files.IsDir(filepath.Join(path, cmd)) { // relative path
		newPath := filepath.Join(path, cmd)
		if s.dir[s.dirIndex] != newPath {
			s.setPath(newPath)
			return true, false, nil
		}
		return false, false, nil
	}
	if files.IsDir(cmd) { // absolute path
		if s.dir[s.dirIndex] != cmd {
			s.setPath(cmd)
			return true, false, nil
		}
		return false, false, nil
	}
	if files.IsFile(filepath.Join(path, cmd)) { // relative path
		if strings.HasPrefix(cmd, "./") && files.IsExecutableCached(filepath.Join(path, cmd)) {
			args := []string{}
			if strings.Contains(cmd, " ") {
				fields := strings.Split(cmd, " ")
				args = fields[1:]
			}
			output, err := run2(cmd, args, path)
			if err == nil {
				s.drawOutput(output)
			}
			return false, false, err
		}
		return false, true, s.edit(cmd, path)
	}
	if files.IsFile(cmd) { // abs absolute path
		return false, true, s.edit(cmd, path)
	}
	if cmd == "l" || cmd == "ls" || cmd == "dir" {
		return false, false, s.ls(path)
	}
	if strings.HasSuffix(cmd, "which ") {
		rest := ""
		if len(cmd) > 6 {
			rest = cmd[6:]
			found := files.WhichCached(rest)
			s.drawOutput(found)
		}
		return false, false, nil
	}
	if cmd == "cd" || cmd == "-" || strings.HasPrefix(cmd, "cd ") {
		possibleDirectory := ""
		rest := ""
		if len(cmd) > 3 {
			rest = strings.TrimSpace(cmd[3:])
			possibleDirectory = filepath.Join(s.dir[s.dirIndex], rest)
		}
		if possibleDirectory == "" && cmd != "-" {
			homedir := env.HomeDir()
			if s.dir[s.dirIndex] != homedir {
				s.setPath(homedir)
				return true, false, nil
			}
			return false, false, nil
		} else if files.IsDir(possibleDirectory) {
			if s.dir[s.dirIndex] != possibleDirectory {
				s.setPath(possibleDirectory)
				return true, false, nil
			}
			return false, false, nil
		} else if files.IsDir(rest) {
			if s.dir[s.dirIndex] != rest {
				s.setPath(rest)
				return true, false, nil
			}
			return false, false, nil
		} else if cmd == "-" || rest == "-" {
			if s.dir[s.dirIndex] != s.prevdir[s.dirIndex] {
				s.prevdir[s.dirIndex], s.dir[s.dirIndex] = s.dir[s.dirIndex], s.prevdir[s.dirIndex]
				return true, false, nil
			}
			return false, false, nil
		}
		return false, false, errors.New("cd WHAT?")
	}
	if cmd == "echo" {
		return false, false, nil
	}
	if strings.HasPrefix(cmd, "echo ") {
		s.drawOutput(cmd[5:])
		return false, false, nil
	}
	if cmd == filepath.Base(env.Str("EDITOR")) {
		return false, true, s.edit("", path)
	}
	if strings.HasPrefix(cmd, filepath.Base(env.Str("EDITOR"))+" ") {
		spaceIndex := strings.Index(cmd, " ")
		rest := ""
		if spaceIndex+1 < len(cmd) {
			rest = cmd[spaceIndex+1:]
		}
		return false, true, s.edit(rest, path)
	}
	if strings.Contains(cmd, " ") {
		fields := strings.Fields(cmd)
		output, err := run2(fields[0], strings.Split(fields[1], " "), s.dir[s.dirIndex])
		if err == nil {
			s.drawOutput(output)
		}
		return false, false, err
	} else if foundExecutableInPath := files.WhichCached(cmd); foundExecutableInPath != "" {
		return false, false, run(foundExecutableInPath, []string{}, s.dir[s.dirIndex])
	}

	return false, false, fmt.Errorf("WHAT DO YOU MEAN, %s?", cmd)
}

func (s *State) currentAbsDir() string {
	path := s.dir[s.dirIndex]
	if absPath, err := filepath.Abs(path); err == nil { // success
		return absPath
	}
	return path
}

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
	cleanupFunc := func() {
		vt.SetXY(0, c.H()-1)
		c.Clear()
		vt.SetLineWrap(true)
		vt.ShowCursor(true)
		//vt.Home() // also clears the screen?
	}
	defer cleanupFunc()

	// Handle ctrl-c
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		cleanupFunc()
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
			c:          c,
			dir:        []string{".", env.HomeDir(), "/tmp"},
			prevdir:    []string{".", env.HomeDir(), "/tmp"},
			dirIndex:   0,
			quit:       false,
			startx:     uint(5),
			starty:     uint(6),
			showHidden: false,
		}
	)

	drawPrompt := func() {
		prompt := ""
		if absPath, err := filepath.Abs(s.dir[s.dirIndex]); err == nil { // success
			prompt = absPath //+ "> "
		} else {
			prompt = s.dir[s.dirIndex] //+ "> "
		}
		prompt = strings.Replace(prompt, env.HomeDir(), "~", 1)
		c.Write(s.startx, s.starty, promptColor, vt.BackgroundDefault, prompt)
		s.promptLength = ulen([]rune(prompt)) + 2 // +2 for > and " "
		c.WriteRune(s.startx+s.promptLength-2, s.starty, angleColor, vt.BackgroundDefault, '>')
		c.WriteRune(s.startx+s.promptLength-1, s.starty, vt.Default, vt.BackgroundDefault, ' ')
	}

	// The rune index for the text that has been written
	index := uint(0)

	drawWritten := func() {
		x = s.startx + s.promptLength
		y = s.starty
		c.Write(x, y, vt.LightYellow, vt.BackgroundDefault, string(s.written))
		r := rune(' ')
		if index < ulen(s.written) {
			r = s.written[index]
		}
		c.WriteRune(x+index, y, vt.Black, vt.BackgroundGreen, r)
		vt.SetXY(x, y)
	}

	clearWritten := func() {
		y := s.starty
		for x := s.startx + s.promptLength; x < c.W(); x++ {
			c.WriteRune(x, y, vt.LightYellow, vt.BackgroundDefault, ' ')
		}
		vt.SetXY(x, y)
	}

	clearAndPrepare := func() {
		c.Clear()

		// the header
		c.Write(5, 2, headerColor, vt.BackgroundDefault, startMessage)

		// the directory number
		c.Write(5, 3, vt.LightYellow, vt.BackgroundDefault, fmt.Sprintf("%d [%s]", s.dirIndex, s.dir[s.dirIndex]))

		// if files are hidden or not
		if s.showHidden {
			c.Write(5, 4, vt.Default, vt.BackgroundDefault, ".")
		} else {
			c.Write(5, 4, vt.Default, vt.BackgroundDefault, " ")
		}

		// the prompt and written text (if any)
		drawPrompt()
		x = s.startx + s.promptLength
		y = s.starty
		drawWritten()
	}

	listDirectory := func() {
		clearAndPrepare()
		s.ls(s.dir[s.dirIndex])
		s.written = []rune{}
		index = 0
		clearWritten()
		drawWritten()
	}

	clearAndPrepare()
	s.ls(s.dir[s.dirIndex])
	c.Draw()

	for !s.quit {
		key := tty.String()
		switch key {
		case "c:27", "c:17": // esc, ctrl-q
			s.quit = true
		case "c:13": // return
			if len(s.written) == 0 {
				listDirectory()
				break
			}
			clearAndPrepare()
			if changedDirectory, editedFile, err := s.execute(string(s.written), s.dir[s.dirIndex]); err != nil {
				s.drawError(err.Error())
			} else if changedDirectory || editedFile {
				listDirectory()
			}
			s.written = []rune{}
			index = 0
			clearWritten()
			drawWritten() // for the cursor
		case "c:127": // backspace
			clearWritten()
			if len(s.written) > 0 && index > 0 {
				s.written = append(s.written[:index-1], s.written[index:]...)
				index--
			}
			drawWritten()
		case "c:11": // ctrl-k
			clearWritten()
			if len(s.written) > 0 {
				s.written = s.written[:index]
			}
			drawWritten()
		case "c:4": // ctrl-d
			if len(s.written) == 0 {
				cleanupFunc()
				os.Exit(1)
				break
			}
			if len(s.written) > 0 {
				clearWritten()
				s.written = append(s.written[:index], s.written[index+1:]...)
				drawWritten()
			}
		case "c:1", homeKey, upArrow: // ctrl-a, home, arrow up
			clearWritten()
			index = 0
			drawWritten()
		case "c:5", endKey, downArrow: // ctrl-e, end, arrow down
			clearWritten()
			index = ulen(s.written) // one after the text
			drawWritten()
		case leftArrow:
			clearWritten()
			if index > 0 {
				index--
			}
			drawWritten()
		case rightArrow:
			clearWritten()
			if index < ulen(s.written) {
				index++
			}
			drawWritten()
		case "c:15", "c:8": // ctrl-o, ctrl-h
			s.showHidden = !s.showHidden
			listDirectory()
		case "c:9": // tab
			if len(s.written) == 0 {
				s.dirIndex++
				if s.dirIndex >= ulen(s.dir) {
					s.dirIndex = 0
				}
				listDirectory()
				break
			}
			clearWritten()
			lastWordWrittenSoFar := strings.TrimPrefix(string(s.written), "./")
			if fields := strings.Fields(lastWordWrittenSoFar); len(fields) > 1 {
				lastWordWrittenSoFar = fields[len(fields)-1]
			}
			found := false
			if entries, err := os.ReadDir(s.dir[s.dirIndex]); err == nil { // success
				for _, entry := range entries {
					name := entry.Name()
					if strings.HasPrefix(name, lastWordWrittenSoFar) {
						rest := []rune(name)[len(lastWordWrittenSoFar):]
						s.written = append(s.written, rest...)
						index += ulen(rest)
						found = true
						break
					}
				}
			}
			if !found {
			OUT:
				for _, p := range env.Path() {
					if entries, err := os.ReadDir(p); err == nil { // success
						for _, entry := range entries {
							name := entry.Name()
							if strings.HasPrefix(name, lastWordWrittenSoFar) && files.IsExecutable(filepath.Join(p, name)) && len(s.written) < len([]rune(name)) {
								rest := []rune(name)[len(s.written):]
								s.written = append(s.written, rest...)
								index += ulen(rest)
								break OUT
							}
						}
					}
				}
			}
			drawWritten()
		case "c:12": // ctrl-l
			c.Clear()
			clearAndPrepare()
			s.ls(s.dir[s.dirIndex])
		case "c:0": // ctrl-space
			run("tig", []string{}, s.dir[s.dirIndex])
		case "c:3": // ctrl-c
			if len(s.written) == 0 {
				cleanupFunc()
				os.Exit(1)
				break
			}
			s.written = []rune{}
			index = 0
			clearWritten()
			drawWritten() // for the cursor
		case "":
			continue
		default:
			if key != " " && strings.TrimSpace(key) == "" {
				continue
			}
			clearWritten()
			tmp := append(s.written[:index], []rune(key)...)
			s.written = append(tmp, s.written[index:]...)
			index += ulen([]rune(key))
			clearWritten()
			drawWritten()
		}
		c.Draw()
	}

	// Write the current directory path to stderr at exit
	fmt.Fprintln(os.Stderr, s.currentAbsDir())
}
