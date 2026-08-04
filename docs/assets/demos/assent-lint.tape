# Lint a conformant example pack (E3-S01 / E9-S10).
# Render: from repo root — vhs docs/assets/demos/assent-lint.tape
Output docs/assets/demos/assent-lint.gif

Set Shell "bash"
Set FontSize 14
Set Width 1000
Set Height 640
Set Theme "Catppuccin Mocha"
Set TypingSpeed 45ms
Set PlaybackSpeed 1.0

Type "# assent lint — hard-error checks over .assent/**"
Sleep 2s
Ctrl+L

Type "task build"
Enter
Sleep 3s
Ctrl+L

Type "./bin/assent lint examples/packs/service-catalog"
Enter
Sleep 4s

Type "# clean pack → exit 0; diagnostics print on failure"
Sleep 2s
