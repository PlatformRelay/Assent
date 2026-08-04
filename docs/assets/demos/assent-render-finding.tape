# Render a committed finding-thread fixture (E8-S10 / E9-S10).
# Render: from repo root — vhs docs/assets/demos/assent-render-finding.tape
Output docs/assets/demos/assent-render-finding.gif

Set Shell "bash"
Set FontSize 14
Set Width 1000
Set Height 640
Set Theme "Catppuccin Mocha"
Set TypingSpeed 45ms
Set PlaybackSpeed 1.0

Type "# assent render — finding-thread markdown from a fixture"
Sleep 2s
Ctrl+L

Type "task build"
Enter
Sleep 3s
Ctrl+L

Type "./bin/assent render --finding examples/render/block"
Enter
Sleep 10s

Type "# summary artifact: add --artifact summary"
Sleep 2s
