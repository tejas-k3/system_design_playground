package scenarios

// The catalog is registered section by section, in this order. Registration order is observable (All() sorts
// stably by difficulty), so keep the calls in the order of the defs_*.go files they come from.
func init() {
	registerFundamentals()
	registerLoadAndLiveData()
	registerFailureAndElasticity()
	registerResourceLimits()
	registerConsistencyAndStreams()
	registerServicePatterns()
	registerScaleOut()
	registerResilienceAndCapstones()
	registerStorage()
}
