package engine

import "sort"

// LeakWindowSize is the fixed size of the rolling "leaked N of last M
// enemies resolved" window exposed via MatchResult.RecentLeaks. It replaces
// a momentum bar in the redesigned UI, so the design deliberately renders
// "leaked none yet" instead of a ratio until the window has actually seen
// this many resolutions -- see RecentLeaks.
const LeakWindowSize = 8

// recordResolution appends one enemy resolution (killed or leaked) to the
// rolling LeakWindow, evicting the oldest entry once the window is full, and
// bumps LeakWindowTotal unconditionally. It is called exactly once per enemy
// that leaves g.Enemies -- from the tower-kill branch in runTowerPhase and
// the breach branch in UpdateGameState -- so LeakWindowTotal always equals
// total kills plus total breaches.
func (g *Game) recordResolution(leaked bool) {
	if g == nil {
		return
	}
	g.LeakWindowTotal++
	g.LeakWindow = append(g.LeakWindow, leaked)
	if len(g.LeakWindow) > LeakWindowSize {
		g.LeakWindow = g.LeakWindow[len(g.LeakWindow)-LeakWindowSize:]
	}
}

// WaveSummary aggregates one wave's outcome for the per-wave timeline: how
// many enemies were sent, how many resolved as a leak, how many were
// killed, a LivesStart->LivesEnd pair, a tower-count snapshot, and whether
// the wave is finished. Wave is g.Wave's own numbering (0-based; wave 0
// collects anything spawned before the first spawnWave() call succeeds).
//
// Complete is true once the wave is no longer current (Wave < g.Wave) --
// nothing can ever tag a new enemy into a wave that has been superseded, see
// Enemy.WaveNumber, so "complete" here means "closed to new arrivals", not
// "every enemy sent into it has resolved". A superseded wave can still have
// stragglers: Sent-Killed-Leaked is the number of that wave's enemies that
// were still alive when the match moved past it (or ended). That identity
// holds for every wave, complete or not; only the still-current wave can see
// it change further.
//
// Leaked counts every enemy that reached the end of its path, full stop --
// including the rare case where the match was already decided earlier in
// the same tick (a second enemy reaching the end after the first one's
// breach already dropped Lives to zero; see the g.GameOver branch in
// UpdateGameState). That one does not cost a life: Lives is deliberately
// not decremented twice for a match that is already over. LivesLost counts
// only resolutions that actually did cost a life -- it is incremented at
// the single g.Lives[g.Defender]-- site, nowhere else, so it cannot drift
// from what actually happened to Lives no matter what other branches do
// later. Leaked and LivesLost are equal on every wave except one that ends
// the match with simultaneous arrivals; a caller who wants "how many of
// this wave's leaks didn't cost a life" computes Leaked-LivesLost.
//
// LivesStart/LivesEnd are derived forward from Game.StartingLives -- a
// fixed, known origin -- not reconstructed backward from the current
// (mutable) Lives value: LivesEnd = LivesStart - LivesLost, and the next
// wave's LivesStart is this wave's LivesEnd. An earlier version derived
// starting lives as "current lives + every leak ever recorded", which had
// two problems: it silently assumed every leak cost a life (false, see
// above), and any single wrong input anywhere corrupted every earlier row,
// not just the one it came from, because the walk ran backward from a
// moving endpoint. Walking forward from a fixed origin has neither problem:
// each row's own life drop always equals its own LivesLost, by construction
// rather than by an assumption about how leaks behave.
//
// Towers is still a plain snapshot (not derivable the way lives is, since
// tower count has no per-wave "delta" concept): frozen at the moment the
// wave is superseded (see supersedeWave) once Complete, read live for the
// still-current wave.
type WaveSummary struct {
	Wave       int  `json:"wave"`
	Sent       int  `json:"sent"`
	Leaked     int  `json:"leaked"`
	Killed     int  `json:"killed"`
	LivesLost  int  `json:"lives_lost"`
	Towers     int  `json:"towers"`
	LivesStart int  `json:"lives_start"`
	LivesEnd   int  `json:"lives_end"`
	Complete   bool `json:"complete"`
}

// waveSummary returns the WaveSummary for wave, creating and registering one
// on first touch. wave is always g.Wave at the moment of creation -- every
// caller assigns Enemy.WaveNumber = g.Wave immediately before the first
// recordWaveEvent call that can reach a given wave number, so a brand new
// entry always describes the wave that is current right now. Towers gets a
// reasonable starting snapshot; LivesStart/LivesEnd are left at their zero
// value here since buildWaveSummaries recomputes them for every row on
// every call, walked forward from Game.StartingLives rather than tracked
// incrementally.
func (g *Game) waveSummary(wave int) *WaveSummary {
	if g.WaveSummaries == nil {
		g.WaveSummaries = map[int]*WaveSummary{}
	}
	ws, ok := g.WaveSummaries[wave]
	if !ok {
		ws = &WaveSummary{Wave: wave, Towers: len(g.Towers)}
		g.WaveSummaries[wave] = ws
	}
	return ws
}

// recordWaveEvent bumps one of a wave's Sent/Killed/Leaked/LivesLost
// counters. kind must be "sent", "killed", "leaked", or "lives_lost" --
// callers at the breach site call both "leaked" (always) and "lives_lost"
// (only when Lives actually decremented) for the same enemy; see
// WaveSummary's doc for why those two are not always the same count. It
// does not touch Towers: that is frozen at supersede time (supersedeWave)
// or read live for the current wave (buildWaveSummaries), never
// accumulated per-touch -- a wave's enemies resolve interleaved with other
// waves' enemies, so "value as of the last touch belonging to this wave" is
// not the same thing as "value as of the end of the wave".
func (g *Game) recordWaveEvent(wave int, kind string) {
	if g == nil {
		return
	}
	ws := g.waveSummary(wave)
	switch kind {
	case "sent":
		ws.Sent++
	case "killed":
		ws.Killed++
	case "leaked":
		ws.Leaked++
	case "lives_lost":
		ws.LivesLost++
	}
}

// supersedeWave freezes wave's Towers snapshot at "right now", the moment it
// stops being current. Called from spawnWave immediately after g.Wave
// advances past it. A no-op if wave never got a row (zero enemies ever
// touched it). It does not touch Lives -- LivesStart/LivesEnd are derived
// forward from Game.StartingLives in buildWaveSummaries, not sampled, so
// there is nothing here to freeze.
func (g *Game) supersedeWave(wave int) {
	if g == nil {
		return
	}
	ws, ok := g.WaveSummaries[wave]
	if !ok {
		return
	}
	ws.Towers = len(g.Towers)
}

// buildWaveSummaries returns g.WaveSummaries as a slice ordered by wave
// number. Complete and, for the still-current wave, Towers are computed
// fresh here (see WaveSummary's doc). LivesStart/LivesEnd are walked
// forward from Game.StartingLives, subtracting each wave's own LivesLost in
// order -- a pure function of data already on Game, anchored at a fixed
// origin rather than reconstructed from a value that keeps changing as the
// match plays out.
func (g *Game) buildWaveSummaries() []WaveSummary {
	if len(g.WaveSummaries) == 0 {
		return nil
	}
	waves := make([]int, 0, len(g.WaveSummaries))
	for w := range g.WaveSummaries {
		waves = append(waves, w)
	}
	sort.Ints(waves)

	lives := g.StartingLives
	out := make([]WaveSummary, 0, len(waves))
	for _, w := range waves {
		ws := *g.WaveSummaries[w]
		ws.Complete = w < g.Wave
		if !ws.Complete {
			ws.Towers = len(g.Towers)
		}
		ws.LivesStart = lives
		lives -= ws.LivesLost
		ws.LivesEnd = lives
		out = append(out, ws)
	}
	return out
}
