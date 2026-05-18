package steps

type Step interface {
	Execute(int) (int, error)
	GetDescription(int) string
}
