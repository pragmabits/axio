//go:build race

package axio

// raceDetector reports whether the tests run under the race detector, which
// drops sync.Pool items at random and so changes what allocates.
const raceDetector = true
