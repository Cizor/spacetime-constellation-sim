package core

import "testing"

func TestTransceiverCompatibility(t *testing.T) {
	a := &TransceiverModel{
		ID: "a",
		Band: FrequencyBand{MinGHz: 10, MaxGHz: 15},
	}
	b := &TransceiverModel{
		ID: "b",
		Band: FrequencyBand{MinGHz: 14, MaxGHz: 18},
	}
	c := &TransceiverModel{
		ID: "c",
		Band: FrequencyBand{MinGHz: 16, MaxGHz: 20},
	}

	if !a.IsCompatible(b) {
		t.Error("a and b should be compatible")
	}
	if a.IsCompatible(c) {
		t.Error("a and c should not be compatible")
	}
}
