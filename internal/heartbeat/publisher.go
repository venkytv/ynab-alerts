package heartbeat

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	hb "github.com/venkytv/nats-heartbeat/pkg/heartbeat"

	"ynab-alerts/internal/config"
)

// Start begins publishing heartbeats until the context is canceled.
func Start(ctx context.Context, cfg config.HeartbeatConfig) (func(), error) {
	if !cfg.Enabled && strings.TrimSpace(cfg.NATSURL) == "" && strings.TrimSpace(cfg.Subject) == "" {
		return nil, nil
	}

	nc, err := nats.Connect(cfg.NATSURL, nats.Name("ynab-alerts heartbeat"))
	if err != nil {
		return nil, err
	}
	pub := hb.NewPublisher(nc, cfg.Prefix)

	runCtx, cancel := context.WithCancel(ctx)
	r := &runner{
		cfg:       cfg,
		publisher: pub,
	}
	go r.loop(runCtx)

	return func() {
		cancel()
		nc.Close()
	}, nil
}

type runner struct {
	cfg       config.HeartbeatConfig
	publisher *hb.Publisher
}

func (r *runner) loop(ctx context.Context) {
	interval := r.cfg.Interval
	if interval <= 0 {
		interval = config.DefaultHeartbeatInterval()
		r.cfg.Interval = interval
	}

	grace := "none"
	if r.cfg.GracePeriod != nil {
		grace = r.cfg.GracePeriod.String()
	}
	skippable := "none"
	if r.cfg.Skippable != nil {
		skippable = strconv.Itoa(*r.cfg.Skippable)
	}

	fullSubject := r.cfg.Subject
	if strings.TrimSpace(r.cfg.Prefix) != "" {
		fullSubject = strings.TrimSuffix(r.cfg.Prefix, ".") + "." + r.cfg.Subject
	}
	log.Printf("heartbeat enabled: publishing %s every %s (grace=%s skippable=%s)", fullSubject, interval, grace, skippable)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	r.publish(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.publish(ctx)
		}
	}
}

func (r *runner) publish(ctx context.Context) {
	msg := hb.Message{
		Subject:     strings.TrimSpace(r.cfg.Subject),
		Interval:    r.cfg.Interval,
		Description: strings.TrimSpace(r.cfg.Description),
		Skippable:   r.cfg.Skippable,
		GracePeriod: r.cfg.GracePeriod,
	}
	if msg.Description == "" {
		msg.Description = msg.Subject
	}

	if err := r.publisher.Publish(ctx, msg); err != nil {
		log.Printf("heartbeat publish failed: %v", err)
	}
}
