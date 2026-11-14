// Command server is the composition root: the only place that knows every module.
package main

import (
	"log/slog"
	"os"

	"github.com/KPV2004/samnak-chat/apps/server/internal/app"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	_ = app.NewRegistry( /* modules are added here as they land */ )
	logger.Info("samnak-chat server skeleton: startup wiring arrives in M0-03..M0-05")
}
