package main

const usageString = versionString + `

Commands within MegaCLI:

any filename        edit file with $EDITOR
cd, .. or any dir   change directory
./script.sh         execute a script named script.sh
l or dir            list directory (happens automatically, though)
q, quit or exit     exit program

Hotkeys:

tab                 cycle between the 3 different current diretories,
                    or tab completion of directories and filenames.
ctrl-space          cycle to the previous directory
ctrl-q              exit program
ctrl-h or ctrl-o    toggle "show hidden files"
ctrl-a              start of line
ctrl-d              delete character under cursor, or exit program
ctrl-k              delete text to the end of the line
ctrl-l              clear screen
ctrl-c              clear text, or exit program
ctrl-t              run tig
ctrl-g              run lazygit
ctrl-n              enter the freshest directory
ctrl-p              go up one directory

Flags:

-v, --version       display the current version
-h, --help          display this help
`
