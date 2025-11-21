package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"ynab-alerts/internal/config"
	"ynab-alerts/internal/notifier"
	"ynab-alerts/internal/rules"
	"ynab-alerts/internal/ynab"
)

// Service orchestrates polling YNAB, evaluating rules, and sending alerts.
type Service struct {
	cfg        config.Config
	ynab       *ynab.Client
	notifier   notifier.Notifier
	ruleStore  *rules.Store
	ruleDir    string
	pollPeriod time.Duration
}

// New builds a Service.
func New(cfg config.Config, ynabClient *ynab.Client, notify notifier.Notifier, store *rules.Store) *Service {
	return &Service{
		cfg:        cfg,
		ynab:       ynabClient,
		notifier:   notify,
		ruleStore:  store,
		ruleDir:    cfg.RulesDir,
		pollPeriod: cfg.PollInterval,
	}
}

// Run starts the polling loop until context cancellation.
func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.pollPeriod)
	defer ticker.Stop()

	// trigger immediately on startup
	s.debugf("starting daemon with poll interval %s", s.pollPeriod)
	if err := s.tick(ctx); err != nil {
		log.Printf("initial tick error: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.tick(ctx); err != nil {
				log.Printf("tick error: %v", err)
			}
		}
	}
}

func (s *Service) tick(ctx context.Context) error {
	now := time.Now()
	if !s.withinEvalWindow(now) {
		s.debugf("skipping evaluation outside window (%s-%s)", s.windowStr())
		return nil
	}

	s.debugf("fetching accounts for budget %s", s.cfg.BudgetID)
	accounts, err := s.ynab.GetAccounts(ctx, s.cfg.BudgetID)
	if err != nil {
		return err
	}
	accountBalances := ynab.BalanceMap(accounts)
	s.debugf("loaded %d account balances", len(accountBalances))

	ruleDefs, err := rules.LoadDir(s.ruleDir)
	if err != nil {
		return err
	}
	s.debugf("loaded %d rule(s)", len(ruleDefs))

	data := rules.Data{
		Accounts: accountBalances,
		Vars:     map[string]int64{},
		Now:      now,
	}
	if s.ruleStore != nil {
		data.Vars = s.ruleStore.Snapshot()
		s.debugf("preloaded %d observed variable(s)", len(data.Vars))
	}

	triggers, err := rules.Evaluate(ctx, ruleDefs, s.ruleStore, data)
	if err != nil {
		return err
	}

	for _, trig := range triggers {
		s.debugf("notifying for rule %s: %s", trig.Rule.Name, trig.Message)
		if err := s.notifier.Notify(ctx, trig.Rule.Name, trig.Message); err != nil {
			log.Printf("notify failed for %s: %v", trig.Rule.Name, err)
		}
	}
	log.Printf("evaluated %d rule(s); %d triggered", len(ruleDefs), len(triggers))
	return nil
}

func (s *Service) debugf(format string, args ...interface{}) {
	if !s.cfg.Debug {
		return
	}
	log.Printf("[debug] "+format, args...)
}

func (s *Service) withinEvalWindow(now time.Time) bool {
	// No window configured.
	if s.cfg.DayStart == 0 && s.cfg.DayEnd == 0 {
		return true
	}
	todayOffset := time.Duration(now.Hour())*time.Hour +
		time.Duration(now.Minute())*time.Minute +
		time.Duration(now.Second())*time.Second
	if s.cfg.DayStart > 0 && todayOffset < s.cfg.DayStart {
		return false
	}
	if s.cfg.DayEnd > 0 && todayOffset >= s.cfg.DayEnd {
		return false
	}
	return true
}

func (s *Service) windowStr() string {
	format := func(d time.Duration) string {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		return fmt.Sprintf("%02d:%02d", h, m)
	}
	if s.cfg.DayStart == 0 && s.cfg.DayEnd == 0 {
		return "none"
	}
	return fmt.Sprintf("%s-%s", format(s.cfg.DayStart), format(s.cfg.DayEnd))
}
