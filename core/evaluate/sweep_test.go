package evaluate_test

import "testing"

func sweepStride(t *testing.T) int {
	if raceDetector || testing.Short() {
		t.Logf("sampling every 101st world of the sweep (race detector %v, -short %v); run without both for all of them", raceDetector, testing.Short())
		return 101
	}
	return 1
}
