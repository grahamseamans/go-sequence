# Project — save & load

> User-perspective UX. The target experience, not what's built. Mechanics
> (serialization, atomic writes) live in `ARCHITECTURE/`.

Project save/load is pure utility — opening the hood while the car is parked. Once
a performance is running you should almost never be in here, and the design should
make sure you don't end up in a file browser when you meant to be playing.

## What you can do

- **Save** the current project to a named file
- **Load** a saved project
- **Rename** a save
- **Delete** a save

Projects are plain files (one per project). A project is the whole instrument
state — tracks, devices, patterns, settings, tempo — saved and restored as one
thing. (There's no separate "runtime state vs. saved state": what you built is
what loads back.)

## The interface

The terminal is the home for this — a list of saves you move through with `j`/`k`,
`enter` to load, `s` to save (type a name), `r` to rename, `d` to delete. Naming
happens in a small edit-mode prompt.

Loading swaps the whole project in place and stops the transport — you're
deliberately leaving the performance to pick up a different one.

## Live or not?

**Tucked away on purpose.** The browser is pre-show / between-songs, not a
performance surface — the Launchpad view stays minimal (or guarded) so you can't
accidentally drop into a file list mid-set.

The one live-relevant affordance is a **quick-save**: a single keystroke that
silently snapshots the current state without ever opening the browser, so you can
capture a good moment without leaving the performance. Loading, renaming, and
deleting are deliberately *not* something you can trigger by accident while the
transport runs.
