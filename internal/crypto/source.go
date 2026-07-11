package crypto

import (
	"context"

	"github.com/find-assets/scanner/internal/model"
)

type Source interface {
	Name() string
	ListAssets(ctx context.Context) ([]Asset, error)
	Klines(ctx context.Context, asset Asset, interval string, limit int) ([]model.Kline, error)
}
