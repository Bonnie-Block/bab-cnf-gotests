package ranptpparameters

// Struct that matches the E810 Plugin Settings.
type E810PluginSettings struct {
	// Holdover state timeout; how long holdover will be.
	LocalHoldoverTimeout uint
	// The maximum offset which will be reached to during Holdover.
	LocalMaxHoldoverOffset uint
	// The maximum offset which is allowed.
	MaxInSpecOffset uint
}
