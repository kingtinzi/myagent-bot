// PinchBot - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 PinchBot contributors

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/sipeed/pinchbot/cmd/picoclaw/internal"
	"github.com/sipeed/pinchbot/cmd/picoclaw/internal/agent"
	"github.com/sipeed/pinchbot/cmd/picoclaw/internal/auth"
	"github.com/sipeed/pinchbot/cmd/picoclaw/internal/check"
	"github.com/sipeed/pinchbot/cmd/picoclaw/internal/cron"
	"github.com/sipeed/pinchbot/cmd/picoclaw/internal/gateway"
	"github.com/sipeed/pinchbot/cmd/picoclaw/internal/migrate"
	"github.com/sipeed/pinchbot/cmd/picoclaw/internal/onboard"
	"github.com/sipeed/pinchbot/cmd/picoclaw/internal/skills"
	"github.com/sipeed/pinchbot/cmd/picoclaw/internal/status"
	"github.com/sipeed/pinchbot/cmd/picoclaw/internal/version"
)

func NewPinchBotCommand() *cobra.Command {
	short := fmt.Sprintf("%s PinchBot - Personal AI Assistant v%s\n\n", internal.Logo, internal.GetVersion())

	cmd := &cobra.Command{
		Use:     "PinchBot",
		Short:   short,
		Example: "PinchBot version",
	}

	cmd.AddCommand(
		onboard.NewOnboardCommand(),
		agent.NewAgentCommand(),
		auth.NewAuthCommand(),
		check.NewCheckCommand(),
		gateway.NewGatewayCommand(),
		status.NewStatusCommand(),
		cron.NewCronCommand(),
		migrate.NewMigrateCommand(),
		skills.NewSkillsCommand(),
		version.NewVersionCommand(),
	)

	return cmd
}

const (
	colorBlue = "\033[1;38;2;62;93;185m"
	colorRed  = "\033[1;38;2;213;70;70m"
	banner    = "\r\n" +
		colorBlue + "██████╗ ██╗ ██████╗ ██████╗ " + colorRed + " ██████╗██╗      █████╗ ██╗    ██╗\n" +
		colorBlue + "██╔══██╗██║██╔════╝██╔═══██╗" + colorRed + "██╔════╝██║     ██╔══██╗██║    ██║\n" +
		colorBlue + "██████╔╝██║██║     ██║   ██║" + colorRed + "██║     ██║     ███████║██║ █╗ ██║\n" +
		colorBlue + "██╔═══╝ ██║██║     ██║   ██║" + colorRed + "██║     ██║     ██╔══██║██║███╗██║\n" +
		colorBlue + "██║     ██║╚██████╗╚██████╔╝" + colorRed + "╚██████╗███████╗██║  ██║╚███╔███╔╝\n" +
		colorBlue + "╚═╝     ╚═╝ ╚═════╝ ╚═════╝ " + colorRed + " ╚═════╝╚══════╝╚═╝  ╚═╝ ╚══╝╚══╝\n " +
		"\033[0m\r\n"
)

func main() {
	fmt.Printf("%s", banner)
	cmd := NewPinchBotCommand()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
