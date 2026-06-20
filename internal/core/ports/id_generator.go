package ports

type IDGenerator interface {
	NewID(prefix string) string
}
