package main

const usageString = versionString + `

Commands within MegaFile:

any filename        edit file with $EDITOR
cd, .. or any dir   change directory
./script.sh         execute a script named script.sh
l or dir            list directory (happens automatically, though)
q, quit or exit     exit program

Hotkeys:

Navigation and Selection:
  arrow keys        navigate and select files (up/down/left/right)
  page up/down      jump to first/last entry in current column
  home or ctrl-a    jump to first file (or start of line when typing)
  end or ctrl-e     jump to last file (or end of line when typing)

Execution:
  return            execute selected file, or run typed command
  esc               clear selection (first press), exit program (second press)

Text Editing:
  backspace         delete character, or go up directory (when at start)
  ctrl-h            delete character, or toggle hidden files (when at start)
  ctrl-d            delete character under cursor, or exit program
  ctrl-k            delete text to the end of the line
  ctrl-c            clear text, or exit program

File Operations:
  tab               cycle through files, or tab completion
  ctrl-f            search for text in files
  ctrl-r            rename file

Directory Navigation:
  ctrl-space        enter the most recent subdirectory
  ctrl-n            cycle to next directory
  ctrl-p            cycle to previous directory
  ctrl-b            go to parent directory
  ctrl-w            go to the real directory (resolve symlinks)

Display:
  ctrl-h            toggle hidden files
  ctrl-o            show more information about the selected file
  ctrl-l            clear screen

External Tools:
  ctrl-t            run tig
  ctrl-g            run lazygit

Exit:
  ctrl-q            exit program immediately

Flags:

-v, --version       display the current version
-h, --help          display this help
`
