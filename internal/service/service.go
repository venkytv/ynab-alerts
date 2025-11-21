package service

import (
	"context"
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
	accounts, err := s.ynab.GetAccounts(ctx, s.cfg.BudgetID)
	if err != nil {
		return err
	}
	accountBalances := ynab.BalanceMap(accounts)

	ruleDefs, err := rules.LoadDir(s.ruleDir)
	if err != nil {
		return err
	}

	data := rules.Data{
		Accounts: accountBalances,
		Vars:     map[string]int64{},
		Now:      time.Now(),
	}
	if s.ruleStore != nil {
		data.Vars = s.ruleStore.Snapshot()
	}

	triggers, err := rules.Evaluate(ctx, ruleDefs, s.ruleStore, data)
	if err != nil {
		return err
	}

	for _, trig := range triggers {
		if err := s.notifier.Notify(ctx, trig.Rule.Name, trig.Message); err != nil {
			log.Printf("notify failed for %s: %v", trig.Rule.Name, err)
		}
	}
	log.Printf("evaluated %d rule(s); %d triggered", len(ruleDefs), len(triggers))
	return nil
}
