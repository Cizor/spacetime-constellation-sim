package core

type SimulationEngine struct {
	KB                  *KnowledgeBase
	ConnectivityService *ConnectivityService
	tickListeners       []func(int)
}

func NewSimulationEngine(kb *KnowledgeBase) *SimulationEngine {
	return &SimulationEngine{
		KB:                  kb,
		ConnectivityService: NewConnectivityService(kb),
		tickListeners:       []func(int){},
	}
}

func (se *SimulationEngine) RegisterTickListener(fn func(int)) {
	se.tickListeners = append(se.tickListeners, fn)
}

func (se *SimulationEngine) Run(ticks int) {
	for tick := 0; tick < ticks; tick++ {
		// Scope 1 logic would go here (e.g., position updates)

		// Scope 2 connectivity evaluation
		se.ConnectivityService.UpdateConnectivity()

		// Notify listeners (optional extensibility)
		for _, fn := range se.tickListeners {
			fn(tick)
		}
	}
}
