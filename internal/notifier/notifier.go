package notifier

import (
	"context"
	"errors"
	"fmt"
	"log"
)

// Notifier dispatches alert messages to an output channel.
type Notifier interface {
	Notify(ctx context.Context, subject, message string) error
}

// Options selects the notifier implementation.
type Options struct {
	Kind     string
	Pushover PushoverConfig
}

// Build constructs a notifier based on the configured kind.
func Build(opts Options) (Notifier, error) {
	switch opts.Kind {
	case "", "pushover":
		if opts.Pushover.AppToken == "" || opts.Pushover.UserKey == "" {
			return nil, errors.New("pushover notifier selected but credentials missing")
		}
		return NewPushover(opts.Pushover), nil
	case "log":
		return LogNotifier{}, nil
	default:
		return nil, fmt.Errorf("unknown notifier kind %q", opts.Kind)
	}
}

// LogNotifier writes alerts to the standard logger (useful for development).
type LogNotifier struct{}

func (LogNotifier) Notify(_ context.Context, subject, message string) error {
	log.Printf("[alert] %s: %s", subject, message)
	return nil
}
