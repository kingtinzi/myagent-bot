package commands

import "context"

func reloadCommand() Definition {
	return Definition{
		Name:        "reload",
		Description: "Reload runtime configuration",
		Usage:       "/reload",
		Handler: func(_ context.Context, req Request, rt *Runtime) error {
			if rt == nil || rt.ReloadConfig == nil {
				return req.Reply(unavailableMsg)
			}
			if err := rt.ReloadConfig(); err != nil {
				return req.Reply("Failed to trigger reload: " + err.Error())
			}
			return req.Reply("Reload triggered.")
		},
	}
}
