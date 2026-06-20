package sqlite

type scanner interface {
	Scan(dest ...any) error
}
