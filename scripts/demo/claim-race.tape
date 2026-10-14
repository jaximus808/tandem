# claim-race.tape — renders assets/demo/claim-race.gif (TDM-4).
#
#   vhs scripts/demo/claim-race.tape      # run from the repo root
#
# Needs vhs (brew install vhs), tmux, curl, jq and a tandem binary (see
# scripts/demo/claim-race.sh). The race is real: a throwaway server on port
# 17962 with its data in a temp dir, two workers claiming the same approved task
# at the same instant. Which worker wins varies run to run; the shape does not.

Output assets/demo/claim-race.gif

Set Shell "bash"
Set FontSize 20
Set Width 1000
Set Height 400
Set Padding 18
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set Framerate 20
Set PlaybackSpeed 1.0

# Off camera: start the server, seed a canvas with two approved tasks, open the
# two panes with each worker's command typed but not yet run.
Hide
Type "export RACE_DIR=${TMPDIR:-/tmp}/tandem-claim-race-vhs && scripts/demo/claim-race.sh up >/dev/null && scripts/demo/claim-race.sh split"
Enter
Sleep 3s
Show

Sleep 1.5s
# Panes are synchronized: this one Enter starts both workers at once.
Enter
Sleep 5s

Hide
# Still inside the panes (cwd scripts/demo): stop the server, kill tmux.
Type "./claim-race.sh down"
Enter
Sleep 500ms
