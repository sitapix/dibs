# Render from the repo root:
#   make build && vhs demo/demo.tape
Output demo/demo.gif

Require dibs

Set Shell          "zsh"
# Code Saver is a paid font (https://www.dbanks.design/code-saver).
# Swap for any installed monospace (e.g. "JetBrains Mono", "Fira Code")
# if you're re-rendering and don't have it.
Set FontFamily     "Code Saver"
Set FontSize       22
Set LineHeight     1.2
Set Width          1200
Set Height         750
Set Padding        28
Set Margin         20
Set MarginFill     "#1a1b26"
Set BorderRadius   12
Set Theme          "GitHub Dark"
Set WindowBar      Colorful
Set CursorBlink    true

Set Framerate      50
Set TypingSpeed    60ms
Set WaitTimeout    30s
Set LoopOffset     8%

# Use the repo-local ./dibs so the demo matches the current source.
Hide
Type "export PATH=$PWD:$PATH" Enter
Type "clear" Enter
Sleep 400ms
Show

Type "dibs orbit"
Sleep 400ms
Enter
Wait
Sleep 3000ms

Hide
Type "clear" Enter
Sleep 300ms
Show

Type "dibs --verify lumen"
Sleep 400ms
Enter
Wait
Sleep 3500ms

Hide
Type "clear" Enter
Sleep 300ms
Show

Type "dibs bas.il"
Sleep 400ms
Enter
Wait
Sleep 3000ms
