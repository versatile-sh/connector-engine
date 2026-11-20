package evaluate_test

import (
	"os"
	"testing"
)

func sweepStride(t *testing.T) int {
	if raceDetector || os.Getenv("CN_EXHAUSTIVE") != "1" {
		t.Logf("sampling every 101st world of the sweep (race detector %v); set CN_EXHAUSTIVE=1 and run without -race for all of them", raceDetector)
		return 101
	}
	return 1
}
