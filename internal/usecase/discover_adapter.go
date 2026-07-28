package usecase

// DiscoverFunc wraps a function as ATDiscoverer (e.g. repository.DiscoverATPort).
func DiscoverFunc(fn func(vendor, product string) (string, error)) ATDiscoverer {
	return discoverFunc(fn)
}
