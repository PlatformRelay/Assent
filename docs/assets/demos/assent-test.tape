# Replay the service-catalog example pack under `assent test` (E9-S10).
# Render: from repo root — vhs docs/assets/demos/assent-test.tape
Output docs/assets/demos/assent-test.gif

Set Shell "bash"
Set FontSize 14
Set Width 1000
Set Height 640
Set Theme "Catppuccin Mocha"
Set TypingSpeed 45ms
Set PlaybackSpeed 1.0

Type "# assent test — replay an example pack's fixtures"
Sleep 2s
Ctrl+L

Type "task build"
Enter
Sleep 4s
Ctrl+L

Type "./bin/assent test examples/packs/service-catalog"
Enter
Sleep 12s

Type "# PASS/BLOCK lines are deterministic replay results"
Sleep 2s
