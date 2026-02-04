package lib

import (
	"testing"
)

func TestRandomMinuteRange(t *testing.T) {
	// Verify randomMinute returns values in the valid range [0, 59)
	for i := 0; i < 100; i++ {
		result := randomMinute()
		if result < 0 || result >= 59 {
			t.Errorf("randomMinute() returned %d, expected value in range [0, 59)", result)
		}
	}
}

func TestRandomMinuteNotConstant(t *testing.T) {
	// Verify randomMinute produces varying output (not stuck on a single value).
	// With 20 calls, getting the same value every time is astronomically unlikely
	// if the RNG is properly seeded.
	seen := make(map[int]bool)
	for i := 0; i < 20; i++ {
		seen[randomMinute()] = true
	}
	if len(seen) < 2 {
		t.Errorf("randomMinute() returned the same value %d times in a row, RNG may not be working", 20)
	}
}
