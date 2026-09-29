package injectors

import (
	"context"
	"time"
)

type PacketLossInjector struct{}

func (i *PacketLossInjector) Name() string { return "packet_loss" }

func (i *PacketLossInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return CallNodeChaosInject(ctx, i.Name(), targetNode, duration, params)
}

func (i *PacketLossInjector) Revert(ctx context.Context, targetNode string) error {
	return CallNodeChaosRevert(ctx, targetNode)
}
