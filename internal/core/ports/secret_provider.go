package ports

import "context"

type SecretProvider interface {
	Resolve(ctx context.Context, key string) (string, error)
}
