> DRAFT — written by AI from current code. Edit / rewrite / delete freely.

# save and load

a save is a single json file holding the whole project. one file per save.

where:

    ~/.go-sequence/saves/<name>.json

writes use tmp-file + rename so a crash mid-write doesn't leave a half-written file. listing skips any leftover `*.tmp.json` straggler.

what gets persisted:
- tempo
- the 8 tracks (name, channel, port name, type, kit, and per-device patterns)

what does NOT get persisted (runtime only — reset on every load):
- which pattern is playing / queued on each track (the cursor — see play_loop.md)
- which pad / button has focus
- ui cursor positions (session grid, settings table)
- save browser state

so loading puts every track back on pattern 0 with nothing queued. if you want to resume mid-song you'd press play and re-queue clips yourself.

operations from the save browser (Shift+S to open):
- create new save (type a name, enter commits)
- load (enter)
- rename
- delete

after load: playback pauses, cursors reset, and each track's currently-playing pattern is marked dirty so the compile goroutine has both machine slots ready by the time you hit play.
