package routerconfiguration

import (
	"context"

	"github.com/openperouter/openperouter/internal/frr"
)

type RouterManager interface {
	New(context.Context) (Router, error)
	NodeIndex(ctx context.Context) (int, error)
}

type Router interface {
	TargetNS(ctx context.Context) (string, error)
	HandleNonRecoverableError(ctx context.Context) error
	Updater(ctx context.Context) (frr.Updater, error)
	CanReconcile(ctx context.Context) (bool, error)
}
