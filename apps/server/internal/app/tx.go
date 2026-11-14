package app

import "context"

// TxManager runs fn inside a transaction carried in ctx.
// Nested calls reuse the outer transaction.
type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}
