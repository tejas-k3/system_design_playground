package scenarios

// The catalog is registered section by section, in this order. Registration order is observable (All() sorts
// stably by difficulty), so keep the calls in the order of the defs_*.go files they come from.
func (c *catalog) registerScenarios() {
	c.registerFundamentals()
	c.registerLoadAndLiveData()
	c.registerFailureAndElasticity()
	c.registerDNS()
	c.registerResourceLimits()
	c.registerConsistencyAndStreams()
	c.registerServicePatterns()
	c.registerScaleOut()
	c.registerResilienceAndCapstones()
	c.registerStorage()
}
