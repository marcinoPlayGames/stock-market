package service_test

import (
	"context"
	"errors"
	"testing"

	"stock-market/internal/model"
	"stock-market/internal/repository"
	"stock-market/internal/service"
)

// --- Mock Repository ---

type mockRepo struct {
	buyErr        error
	sellErr       error
	walletStocks  []model.StockEntry
	walletQty     int64
	stockExists   bool
	bankStocks    []model.StockEntry
	logEntries    []model.LogEntry
	appendedLogs  []model.LogEntry
}

func (m *mockRepo) Buy(_ context.Context, _, _ string) error  { return m.buyErr }
func (m *mockRepo) Sell(_ context.Context, _, _ string) error { return m.sellErr }

func (m *mockRepo) GetWallet(_ context.Context, _ string) ([]model.StockEntry, error) {
	return m.walletStocks, nil
}

func (m *mockRepo) GetWalletStock(_ context.Context, _, _ string) (int64, bool, error) {
	return m.walletQty, m.stockExists, nil
}

func (m *mockRepo) GetBankStocks(_ context.Context) ([]model.StockEntry, error) {
	return m.bankStocks, nil
}

func (m *mockRepo) SetBankStocks(_ context.Context, stocks []model.StockEntry) error {
	m.bankStocks = stocks
	return nil
}

func (m *mockRepo) AppendLog(_ context.Context, entry model.LogEntry) error {
	m.appendedLogs = append(m.appendedLogs, entry)
	return nil
}

func (m *mockRepo) GetLog(_ context.Context) ([]model.LogEntry, error) {
	return m.logEntries, nil
}

// --- Trade Tests ---

func TestTrade_Buy_Success(t *testing.T) {
	repo := &mockRepo{}
	svc := service.NewStockService(repo)

	err := svc.Trade(context.Background(), "wallet1", "AAPL", "buy")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(repo.appendedLogs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(repo.appendedLogs))
	}
	if repo.appendedLogs[0].Type != "buy" {
		t.Errorf("expected log type 'buy', got '%s'", repo.appendedLogs[0].Type)
	}
}

func TestTrade_Sell_Success(t *testing.T) {
	repo := &mockRepo{}
	svc := service.NewStockService(repo)

	err := svc.Trade(context.Background(), "wallet1", "AAPL", "sell")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(repo.appendedLogs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(repo.appendedLogs))
	}
	if repo.appendedLogs[0].Type != "sell" {
		t.Errorf("expected log type 'sell', got '%s'", repo.appendedLogs[0].Type)
	}
}

func TestTrade_InvalidType_Returns400(t *testing.T) {
	repo := &mockRepo{}
	svc := service.NewStockService(repo)

	err := svc.Trade(context.Background(), "wallet1", "AAPL", "invalid")

	te, ok := err.(*service.TradeError)
	if !ok {
		t.Fatalf("expected TradeError, got %T", err)
	}
	if te.Code != 400 {
		t.Errorf("expected code 400, got %d", te.Code)
	}
}

func TestTrade_StockNotFound_Returns404(t *testing.T) {
	repo := &mockRepo{buyErr: repository.ErrStockNotFound}
	svc := service.NewStockService(repo)

	err := svc.Trade(context.Background(), "wallet1", "UNKNOWN", "buy")

	te, ok := err.(*service.TradeError)
	if !ok {
		t.Fatalf("expected TradeError, got %T", err)
	}
	if te.Code != 404 {
		t.Errorf("expected code 404, got %d", te.Code)
	}
}

func TestTrade_InsufficientBank_Returns400(t *testing.T) {
	repo := &mockRepo{buyErr: repository.ErrInsufficientBank}
	svc := service.NewStockService(repo)

	err := svc.Trade(context.Background(), "wallet1", "AAPL", "buy")

	te, ok := err.(*service.TradeError)
	if !ok {
		t.Fatalf("expected TradeError, got %T", err)
	}
	if te.Code != 400 {
		t.Errorf("expected code 400, got %d", te.Code)
	}
}

func TestTrade_InsufficientWallet_Returns400(t *testing.T) {
	repo := &mockRepo{sellErr: repository.ErrInsufficientWallet}
	svc := service.NewStockService(repo)

	err := svc.Trade(context.Background(), "wallet1", "AAPL", "sell")

	te, ok := err.(*service.TradeError)
	if !ok {
		t.Fatalf("expected TradeError, got %T", err)
	}
	if te.Code != 400 {
		t.Errorf("expected code 400, got %d", te.Code)
	}
}

func TestTrade_FailedOperation_DoesNotLog(t *testing.T) {
	repo := &mockRepo{buyErr: repository.ErrInsufficientBank}
	svc := service.NewStockService(repo)

	_ = svc.Trade(context.Background(), "wallet1", "AAPL", "buy")

	if len(repo.appendedLogs) != 0 {
		t.Errorf("expected no log entries on failed trade, got %d", len(repo.appendedLogs))
	}
}

func TestTrade_UnknownError_Returns500(t *testing.T) {
	repo := &mockRepo{buyErr: errors.New("unexpected redis error")}
	svc := service.NewStockService(repo)

	err := svc.Trade(context.Background(), "wallet1", "AAPL", "buy")

	te, ok := err.(*service.TradeError)
	if !ok {
		t.Fatalf("expected TradeError, got %T", err)
	}
	if te.Code != 500 {
		t.Errorf("expected code 500, got %d", te.Code)
	}
}

// --- GetWallet Tests ---

func TestGetWallet_ReturnsStocks(t *testing.T) {
	repo := &mockRepo{
		walletStocks: []model.StockEntry{
			{Name: "AAPL", Quantity: 3},
		},
	}
	svc := service.NewStockService(repo)

	resp, err := svc.GetWallet(context.Background(), "wallet1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "wallet1" {
		t.Errorf("expected ID 'wallet1', got '%s'", resp.ID)
	}
	if len(resp.Stocks) != 1 {
		t.Errorf("expected 1 stock, got %d", len(resp.Stocks))
	}
}

func TestGetWallet_EmptyWallet_ReturnsEmptySlice(t *testing.T) {
	repo := &mockRepo{walletStocks: nil}
	svc := service.NewStockService(repo)

	resp, err := svc.GetWallet(context.Background(), "wallet1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Stocks == nil {
		t.Error("expected empty slice, got nil")
	}
}

// --- GetWalletStock Tests ---

func TestGetWalletStock_StockExists(t *testing.T) {
	repo := &mockRepo{walletQty: 5, stockExists: true}
	svc := service.NewStockService(repo)

	qty, tradeErr := svc.GetWalletStock(context.Background(), "wallet1", "AAPL")

	if tradeErr != nil {
		t.Fatalf("unexpected error: %v", tradeErr)
	}
	if qty != 5 {
		t.Errorf("expected quantity 5, got %d", qty)
	}
}

func TestGetWalletStock_StockNotInBank_Returns404(t *testing.T) {
	repo := &mockRepo{stockExists: false}
	svc := service.NewStockService(repo)

	_, tradeErr := svc.GetWalletStock(context.Background(), "wallet1", "UNKNOWN")

	if tradeErr == nil {
		t.Fatal("expected error, got nil")
	}
	if tradeErr.Code != 404 {
		t.Errorf("expected code 404, got %d", tradeErr.Code)
	}
}

// --- GetBankStocks Tests ---

func TestGetBankStocks_ReturnsStocks(t *testing.T) {
	repo := &mockRepo{
		bankStocks: []model.StockEntry{
			{Name: "AAPL", Quantity: 100},
			{Name: "GOOG", Quantity: 50},
		},
	}
	svc := service.NewStockService(repo)

	resp, err := svc.GetBankStocks(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Stocks) != 2 {
		t.Errorf("expected 2 stocks, got %d", len(resp.Stocks))
	}
}

func TestGetBankStocks_Empty_ReturnsEmptySlice(t *testing.T) {
	repo := &mockRepo{}
	svc := service.NewStockService(repo)

	resp, err := svc.GetBankStocks(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Stocks == nil {
		t.Error("expected empty slice, got nil")
	}
}

// --- SetBankStocks Tests ---

func TestSetBankStocks_SetsCorrectly(t *testing.T) {
	repo := &mockRepo{}
	svc := service.NewStockService(repo)

	stocks := []model.StockEntry{{Name: "AAPL", Quantity: 99}}
	err := svc.SetBankStocks(context.Background(), stocks)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.bankStocks) != 1 || repo.bankStocks[0].Name != "AAPL" {
		t.Errorf("bank stocks not set correctly: %v", repo.bankStocks)
	}
}

// --- GetLog Tests ---

func TestGetLog_ReturnsEntries(t *testing.T) {
	repo := &mockRepo{
		logEntries: []model.LogEntry{
			{Type: "buy", WalletID: "w1", StockName: "AAPL"},
			{Type: "sell", WalletID: "w1", StockName: "AAPL"},
		},
	}
	svc := service.NewStockService(repo)

	resp, err := svc.GetLog(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Log) != 2 {
		t.Errorf("expected 2 log entries, got %d", len(resp.Log))
	}
}

func TestGetLog_Empty_ReturnsEmptySlice(t *testing.T) {
	repo := &mockRepo{}
	svc := service.NewStockService(repo)

	resp, err := svc.GetLog(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Log == nil {
		t.Error("expected empty slice, got nil")
	}
}