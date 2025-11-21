package ynab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client wraps minimal YNAB API calls needed for alerting.
type Client struct {
	token   string
	baseURL string
	client  *http.Client
}

// NewClient builds a YNAB client.
func NewClient(token, baseURL string) *Client {
	if baseURL == "" {
		baseURL = "https://api.ynab.com/v1"
	}
	return &Client{
		token:   token,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

// Account holds a subset of YNAB account data.
type Account struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Balance  int64  `json:"balance"`
	Type     string `json:"type"`
	OnBudget bool   `json:"on_budget"`
}

// Budget holds minimal budget info.
type Budget struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	CurrencyFormat *CurrencyFormat `json:"currency_format,omitempty"`
}

// AccountsResponse mirrors the YNAB API response shape.
type AccountsResponse struct {
	Data struct {
		Accounts []Account `json:"accounts"`
	} `json:"data"`
}

// BudgetsResponse mirrors the budgets list shape.
type BudgetsResponse struct {
	Data struct {
		Budgets []Budget `json:"budgets"`
	} `json:"data"`
}

// BudgetDetailResponse mirrors the budget detail shape.
type BudgetDetailResponse struct {
	Data struct {
		Budget BudgetDetail `json:"budget"`
	} `json:"data"`
}

// BudgetDetail holds budget info with currency format.
type BudgetDetail struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	CurrencyFormat *CurrencyFormat `json:"currency_format,omitempty"`
}

// CurrencyFormat describes how to display currency.
type CurrencyFormat struct {
	Symbol           string `json:"currency_symbol"`
	SymbolFirst      bool   `json:"symbol_first"`
	DecimalDigits    int    `json:"decimal_digits"`
	DecimalSeparator string `json:"decimal_separator"`
	GroupSeparator   string `json:"group_separator"`
	ExampleFormat    string `json:"example_format"`
	DisplaySymbol    bool   `json:"display_symbol"`
	ISOCode          string `json:"iso_code"`
}

// GetAccounts fetches all accounts for a budget.
func (c *Client) GetAccounts(ctx context.Context, budgetID string) ([]Account, error) {
	url := fmt.Sprintf("%s/budgets/%s/accounts", c.baseURL, budgetID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ynab accounts request failed: %s", resp.Status)
	}

	var decoded AccountsResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	return decoded.Data.Accounts, nil
}

// GetBudgets fetches budgets available to the token.
func (c *Client) GetBudgets(ctx context.Context) ([]Budget, error) {
	url := fmt.Sprintf("%s/budgets", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ynab budgets request failed: %s", resp.Status)
	}

	var decoded BudgetsResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	return decoded.Data.Budgets, nil
}

// GetBudget fetches a single budget for metadata (currency format).
func (c *Client) GetBudget(ctx context.Context, budgetID string) (*BudgetDetail, error) {
	url := fmt.Sprintf("%s/budgets/%s", c.baseURL, budgetID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ynab budget request failed: %s", resp.Status)
	}

	var decoded BudgetDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	return &decoded.Data.Budget, nil
}

// BalanceMap returns account balances keyed by account name.
func BalanceMap(accounts []Account) map[string]int64 {
	out := make(map[string]int64, len(accounts))
	for _, a := range accounts {
		out[a.Name] = a.Balance
	}
	return out
}
