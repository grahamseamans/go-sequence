##### the clock

one global tick clock. ticks happen at PPQ (pulses per quarter note, currently 960) per beat. tempo (BPM) sets how fast ticks fire in real time.

## human pattern vs machine pattern

each pattern is stored twice.
- the **human pattern** this is called human pattern cause it's what the human edits, but it's really more like, how the actual midi device thinks. it's meant to be like, represenation of the pattern as the device exists, it's pretty similar to being the devices state.
- the **machine pattern** is compiled. a list of {tick offset, midi event} entries plus a Length. it's not how the device sees the pattern, all devices put out this same thing. it's really just like, a giant list that we can look through to send stuff out, you can think of it as a compiled pattern, similar to machine code and compiled code vs whatever pre-compiled code is called

playback only ever reads machine patterns. the compile goroutine is the bridge.

the machine pattern is **double-buffered** **for every midi device, for every pattern**: Machine[0][0] and Machine[0][1]. playback always reads one slot. the compile goroutine fills the used up one once it's finished. the slot that is read from swaps on every pattern loop.

for each pattern in all devices we aim to always have 2 fresh patterns compiled and read to be read from!

##### playhead

each track has a Cursor{CurrentPattern, T0Tick, CurrentSlot, QueuedPattern}.
- T0Tick - global tick when currently-playing pattern started.
- CurrentSlot: which compiled slot (0 or 1) playback is reading right now.

the playhead is NOT stored. it's computed every tick:

    playhead = (globalTick - T0Tick) % patterns[CurrentPattern].patternLength 

^^ or however we sttore pattern length

we use this to:
- lookup where in the computed pattern should be read from
- check if it's time for to loop to the next slot and recompute the last one so that it's ready to go?

##### exaple playback thing:

so you like
are on machine a - you finish it
check if there's a queued pattern, if so swap over
you update the t0 to be previous v0 + a_len
you start reading from machine b, and have the compile go routeing make the next a for you
then when you finish b, check for queued stuf, swap to reading from a, and have b get re-compiled

## edits

if you edit a pattern both of it's compiled patterns get remade right then. if you make the pattern shorter such that the playhead would wrap around then you just wrap around unti lyou get to where you would play from, and read from there.
