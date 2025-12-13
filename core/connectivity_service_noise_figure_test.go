package core

import (
	"testing"
)

// TestAverageNoiseFigure_ZeroValue verifies that a noise figure of 0 dB
// (valid for a perfect receiver) is correctly included in the average,
// not skipped as "unset".
func TestAverageNoiseFigure_ZeroValue(t *testing.T) {
	// Create transceiver with 0 dB noise figure (perfect receiver)
	trxZero := &TransceiverModel{
		ID:   "trx-zero-nf",
		Name: "Perfect Receiver",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
	}
	zeroNF := 0.0
	trxZero.SystemNoiseFigureDB = &zeroNF

	// Create transceiver with 5 dB noise figure
	trxFive := &TransceiverModel{
		ID:   "trx-five-nf",
		Name: "Normal Receiver",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
	}
	fiveNF := 5.0
	trxFive.SystemNoiseFigureDB = &fiveNF

	// Average of 0 dB and 5 dB should be 2.5 dB
	avg := averageNoiseFigure(trxZero, trxFive)
	if avg != 2.5 {
		t.Fatalf("expected average of 0 dB and 5 dB to be 2.5 dB, got %f", avg)
	}

	// Average of 0 dB and 0 dB should be 0 dB
	avg2 := averageNoiseFigure(trxZero, trxZero)
	if avg2 != 0.0 {
		t.Fatalf("expected average of 0 dB and 0 dB to be 0 dB, got %f", avg2)
	}
}

// TestAverageNoiseFigure_Unset verifies that unset (nil) noise figures
// are correctly excluded from the average.
func TestAverageNoiseFigure_Unset(t *testing.T) {
	// Create transceiver with unset noise figure (nil pointer)
	trxUnset := &TransceiverModel{
		ID:   "trx-unset-nf",
		Name: "Receiver Without Noise Figure",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
		// SystemNoiseFigureDB is nil (unset)
	}

	// Create transceiver with 5 dB noise figure
	trxFive := &TransceiverModel{
		ID:   "trx-five-nf",
		Name: "Normal Receiver",
		Band: FrequencyBand{
			MinGHz: 10.0,
			MaxGHz: 12.0,
		},
	}
	fiveNF := 5.0
	trxFive.SystemNoiseFigureDB = &fiveNF

	// Average of unset and 5 dB should be 5 dB (only the set value is used)
	avg := averageNoiseFigure(trxUnset, trxFive)
	if avg != 5.0 {
		t.Fatalf("expected average of unset and 5 dB to be 5 dB, got %f", avg)
	}

	// Average of both unset should be 0 (no values to average)
	avg2 := averageNoiseFigure(trxUnset, trxUnset)
	if avg2 != 0.0 {
		t.Fatalf("expected average of two unset noise figures to be 0, got %f", avg2)
	}
}
