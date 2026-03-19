package qq

import (
	"fmt"
	"strings"

	"github.com/sipeed/pinchbot/pkg/logger"
)

// botGoLogger keeps useful botgo logs while demoting heartbeat noise to DEBUG.
type botGoLogger struct {
	component string
}

func newBotGoLogger(component string) *botGoLogger {
	return &botGoLogger{component: component}
}

func (b *botGoLogger) Debug(v ...any) {
	logger.DebugC(b.component, fmt.Sprint(v...))
}

func (b *botGoLogger) Info(v ...any) {
	message := fmt.Sprint(v...)
	if shouldDemoteBotGoInfo(message) {
		logger.DebugC(b.component, message)
		return
	}
	logger.InfoC(b.component, message)
}

func (b *botGoLogger) Warn(v ...any) {
	logger.WarnC(b.component, fmt.Sprint(v...))
}

func (b *botGoLogger) Error(v ...any) {
	logger.ErrorC(b.component, fmt.Sprint(v...))
}

func (b *botGoLogger) Debugf(format string, v ...any) {
	logger.DebugC(b.component, fmt.Sprintf(format, v...))
}

func (b *botGoLogger) Infof(format string, v ...any) {
	message := fmt.Sprintf(format, v...)
	if shouldDemoteBotGoInfo(message) {
		logger.DebugC(b.component, message)
		return
	}
	logger.InfoC(b.component, message)
}

func (b *botGoLogger) Warnf(format string, v ...any) {
	logger.WarnC(b.component, fmt.Sprintf(format, v...))
}

func (b *botGoLogger) Errorf(format string, v ...any) {
	logger.ErrorC(b.component, fmt.Sprintf(format, v...))
}

func (b *botGoLogger) Sync() error {
	return nil
}

func shouldDemoteBotGoInfo(message string) bool {
	return strings.Contains(message, " write Heartbeat message") ||
		strings.Contains(message, " receive HeartbeatAck message")
}
