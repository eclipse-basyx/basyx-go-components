package steps

// Step defines one executable initialization step for the configuration service.
type Step interface {
	Execute(int) (int, error)
	GetDescription(int) string
}
