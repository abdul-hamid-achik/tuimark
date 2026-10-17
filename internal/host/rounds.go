package host

// MaxRounds is the number of rounds one settle dispatches at most (SPEC
// §15.4 point 3, the v0.1 loop): the events of the input that caused it
// (after any kept from the previous settle, TakeCarry), then the lifecycle
// events each render queues, rendering again between rounds.
const MaxRounds = 8

// TakeCarry returns and clears the lifecycle events the previous settle
// kept at its 8-round cap (SPEC v0.4a §15.4 point 3, CarryOver). The
// settle that takes them dispatches them first, as its first round, ahead
// of the events of the input that caused it (a key, a paste, a mouse
// event, a host write, a resize, the size poll, the esc timeout, an Every
// tick, or Play's step 0), and counts its own 8 rounds from there.
func (a *App) TakeCarry() []Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.carry
	a.carry = nil
	return c
}

// CarryOver is called when a settle's 8th round ends with events still
// queued (SPEC v0.4a §15.4 point 3): taken, the events taken for a 9th
// round, then every lifecycle event queued after them (the render that
// followed that take), are kept, in that order, for the next settle
// instead of being dropped or left behind the next input's own events. It
// returns how many events are kept, for TUIMARK_LOG's rounds record
// (§26.12).
func (a *App) CarryOver(taken []Event) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.carry = append(append(a.carry, taken...), a.pending...)
	a.pending = nil
	return len(a.carry)
}
