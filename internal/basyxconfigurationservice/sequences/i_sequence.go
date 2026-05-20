package steps

// Sequence defines one executable initialization step for the configuration service.
type Sequence interface {
	Execute(int) (int, error)
	GetDescription(int) string
}
