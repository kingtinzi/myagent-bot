package gateway

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/gatewayservice"
)

var newGatewayService = gatewayservice.New

func gatewayCmd(debug bool) error {
	svc, err := newGatewayService(gatewayservice.Options{
		ConfigPath: config.GetConfigPath(),
		Debug:      debug,
		OnLog: func(line string) {
			fmt.Println(line)
		},
	})
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := svc.Start(ctx); err != nil {
		return err
	}

	<-ctx.Done()
	fmt.Println("\nShutting down...")
	if err := svc.Stop(context.Background()); err != nil {
		return err
	}
	fmt.Println("✓ Gateway stopped")
	return nil
}

func ensureWorkspaceBootstrap(workspace string) error {
	return gatewayservice.EnsureWorkspaceBootstrap(workspace)
}
