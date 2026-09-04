package injectors

import (
	"context"
	"fmt"
	"time"
)

type PacketLossInjector struct{}

func (i *PacketLossInjector) Name() string { return "packet_loss" }

func (i *PacketLossInjector) Inject(ctx context.Context, targetNode string, duration time.Duration, params map[string]string) error {
	return fmt.Errorf("packet_loss injector not yet implemented (Phase 1)")
}

func (i *PacketLossInjector) Revert(ctx context.Context, targetNode string) error {
	return nil
}
