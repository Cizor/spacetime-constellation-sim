package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
	"github.com/signalsfoundry/constellation-simulator/timectrl"
)

func main() {
	duration := flag.Duration("duration", 60*time.Second, "total simulation duration")
	tick := flag.Duration("tick", 1*time.Second, "tick interval")
	accelerated := flag.Bool("accelerated", true, "run in accelerated mode (vs real-time)")
	impairLinkID := flag.String(
		"impair-link",
		"",
		"ID of a network link to mark as impaired at startup",
	)
	impairInterfaceID := flag.String(
		"impair-interface",
		"",
		"ID of a network interface to mark as impaired (non-operational) at startup",
	)

	flag.Parse()

	// ==== Scope 1: Core platform + motion setup ====

	store := kb.NewKnowledgeBase()

	sat := &model.PlatformDefinition{
		ID:           "sat1",
		Name:         "LEO-Sat-1",
		Type:         "SATELLITE",
		MotionSource: model.MotionSourceSpacetrack,
	}
	ground := &model.PlatformDefinition{
		ID:           "ground1",
		Name:         "Equator-GS",
		Type:         "GROUND_STATION",
		MotionSource: model.MotionSourceUnknown,
		// ~Earth radius on x-axis (metres)
		Coordinates: model.Motion{X: 6371000, Y: 0, Z: 0},
	}

	if err := store.AddPlatform(sat); err != nil {
		panic(err)
	}
	if err := store.AddPlatform(ground); err != nil {
		panic(err)
	}

	// Network nodes attached to platforms (Scope 1)
	if err := store.AddNetworkNode(&model.NetworkNode{
		ID:         "node-sat1",
		Name:       "SatNode",
		PlatformID: "sat1",
	}); err != nil {
		panic(err)
	}
	if err := store.AddNetworkNode(&model.NetworkNode{
		ID:         "node-ground1",
		Name:       "GroundNode",
		PlatformID: "ground1",
	}); err != nil {
		panic(err)
	}

	// Motion models
	tle1 := "1 25544U 98067A   21275.59097222  .00000204  00000-0  10270-4 0  9990"
	tle2 := "2 25544  51.6459 115.9059 0001817  61.3028  35.9198 15.49370953257760"
	satModel := core.NewMotionModel(sat, tle1, tle2)
	groundModel := core.NewMotionModel(ground, "", "")

	// ==== Scope 2: Network KB + connectivity service ====

	netKB := core.NewKnowledgeBase()

	// Load transceiver models (Ku/Ka/etc.) from JSON.
	loadTransceivers(netKB, "configs/transceivers.json")

	// Load network interfaces + links from JSON scenario, instead of hard-coding.
	scenarioPath := "configs/network_scenario.json"
	f, err := os.Open(scenarioPath)
	if err != nil {
		panic(fmt.Errorf("failed to open network scenario %q: %w", scenarioPath, err))
	}
	defer f.Close()

	netScenario, err := core.LoadNetworkScenario(netKB, f)
	if err != nil {
		panic(fmt.Errorf("failed to load network scenario: %w", err))
	}
	if *impairLinkID != "" {
		if err := netKB.SetLinkImpaired(*impairLinkID, true); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to impair link %q: %v\n", *impairLinkID, err)
		} else {
			fmt.Printf("Impaired link %q at startup\n", *impairLinkID)
		}
	}

	if *impairInterfaceID != "" {
		if err := netKB.SetInterfaceImpaired(*impairInterfaceID, true); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to impair interface %q: %v\n", *impairInterfaceID, err)
		} else {
			fmt.Printf("Impaired interface %q at startup\n", *impairInterfaceID)
		}
	}

	// Optional: one-liner so we know what was loaded.
	fmt.Printf("Loaded network scenario: %d interfaces, %d links, %d nodes with positions\n",
		len(netScenario.InterfaceIDs), len(netScenario.LinkIDs), len(netScenario.NodeIDs))

	connectivity := core.NewConnectivityService(netKB)
	// Optionally tweak:
	// connectivity.MinElevationDeg = 10.0
	// connectivity.DefaultWiredLatencyMs = 5.0

	// ==== Time controller (Scope 1) ====

	mode := timectrl.RealTime
	if *accelerated {
		mode = timectrl.Accelerated
	}
	start := time.Now().UTC()
	tc := timectrl.NewTimeController(start, *tick, mode)

	tc.AddListener(func(simTime time.Time) {
		// --- Update platform positions (Scope 1) ---

		satModel.UpdatePosition(simTime, sat)
		if err := store.UpdatePlatformPosition(sat.ID, sat.Coordinates); err != nil {
			fmt.Printf("update sat position error: %v\n", err)
		}

		groundModel.UpdatePosition(simTime, ground)
		if err := store.UpdatePlatformPosition(ground.ID, ground.Coordinates); err != nil {
			fmt.Printf("update ground position error: %v\n", err)
		}

		// --- Push ECEF positions into network KB (Scope 2) ---
		// Scope 1 uses metres; Scope 2 geometry layer uses km.
		netKB.SetNodeECEFPosition("sat1", core.Vec3{
			X: sat.Coordinates.X / 1000.0,
			Y: sat.Coordinates.Y / 1000.0,
			Z: sat.Coordinates.Z / 1000.0,
		})
		netKB.SetNodeECEFPosition("ground1", core.Vec3{
			X: ground.Coordinates.X / 1000.0,
			Y: ground.Coordinates.Y / 1000.0,
			Z: ground.Coordinates.Z / 1000.0,
		})

		// --- Evaluate connectivity (Scope 2) ---

		connectivity.UpdateConnectivity()

		// --- Logging / demo output ---

		fmt.Printf("[%s] %s @ (%.0f, %.0f, %.0f); %s @ (%.0f, %.0f, %.0f)\n",
			simTime.Format(time.RFC3339),
			sat.ID, sat.Coordinates.X, sat.Coordinates.Y, sat.Coordinates.Z,
			ground.ID, ground.Coordinates.X, ground.Coordinates.Y, ground.Coordinates.Z,
		)

		// Show all links with SNR and quality. Most interesting will
		// be the dynamic wireless sat1–ground1 link.
		for _, link := range netKB.GetAllNetworkLinks() {
			fmt.Printf("↳ Link %-24s [%s ↔ %s] up=%-5v quality=%-10s SNR=%5.1f dB rate=%6.1f Mbps\n",
				link.ID,
				link.InterfaceA,
				link.InterfaceB,
				link.IsUp,
				link.Quality,
				link.SNRdB,
				link.MaxDataRateMbps,
			)
		}
	})

	fmt.Printf("Starting simulation: duration=%s, tick=%s, mode=%v\n", *duration, *tick, mode)
	done := tc.Start(*duration)
	<-done
	fmt.Println("Simulation complete.")
}

// loadTransceivers reads transceiver models from JSON and registers
// them in the network KnowledgeBase. This stays local to main for now;
// it just unmarshals into core.TransceiverModel objects.
func loadTransceivers(kb *core.KnowledgeBase, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var trxs []*core.TransceiverModel
	if err := json.Unmarshal(data, &trxs); err != nil {
		panic(err)
	}
	for _, trx := range trxs {
		kb.AddTransceiverModel(trx)
	}
}
