package service_test

import (
	"context"
	"errors"
	"testing"

	"stock-market/internal/model"
	"stock-market/internal/repository"
	"stock-market/internal/service"
)

// mockRepo implements repository.Repository. Because NewStockService now
// accepts the interface, this mock compiles and is injected directly —
// no real Redis connection required.
type mockRepo struct {
	buyErr       error
	sellErr      error
	walletStocks []model.StockEntry
	walletErr    error
	walletQty    int64
	stockExists  bool
	walletQtyErr error
	bankStocks   []model.StockEntry
	bankErr      error
	logEntries   []model.LogEntry
	logErr       error
	appendedLogs []model.LogEntry
}

func (m *mockRepo) Buy(_ context.Context, _, _ string) error  { return m.buyErr }
func (m *mockRepo) Sell(_ context.Context, _, _ string) error { return m.sellErr }

func (m *mockRepo) GetWallet(_ context.Context, _ string) ([]model.StockEntry, error) {
	return m.walletStocks, m.walletErr
}

func (m *mockRepo) GetWalletStock(_ context.Context, _, _ string) (int64, bool, error) {
	return m.walletQty, m.stockExists, m.walletQtyErr
}

func (m *mockRepo) GetBankStocks(_ context.Context) ([]model.StockEntry, error) {
	return m.bankStocks, m.bankErr
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
	return m.logEntries, m.logErr
}

// compile-time guard: mockRepo must satisfy repository.Repository
var _ repository.Repository = (*mockRepo)(nil)

// helpers

func newSvc(repo *mockRepo) *service.StockService {
	return service.NewStockService(repo)
}

// --- Trade ---

func TestTrade_Buy_Success(t *testing.T) {
	repo := &mockRepo{}
	err := newSvc(repo).Trade(context.Background(), "wallet1", "AAPL", "buy")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(repo.appendedLogs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(repo.appendedLogs))
	}
	log := repo.appendedLogs[0]
	if log.Type != "buy" || log.WalletID != "wallet1" || log.StockName != "AAPL" {
		t.Errorf("unexpected log entry: %+v", log)
	}
}

func TestTrade_Sell_Success(t *testing.T) {
	repo := &mockRepo{}
	err := newSvc(repo).Trade(context.Background(), "wallet1", "AAPL", "sell")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(repo.appendedLogs) != 1 || repo.appendedLogs[0].Type != "sell" {
		t.Errorf("expected sell log entry, got: %+v", repo.appendedLogs)
	}
}

func TestTrade_InvalidType_Returns400(t *testing.T) {
	te := mustTradeError(t, newSvc(&mockRepo{}).Trade(context.Background(), "w1", "AAPL", "hodl"))
	assertCode(t, te, 400)
}

func TestTrade_StockNotFound_Returns404(t *testing.T) {
	repo := &mockRepo{buyErr: repository.ErrStockNotFound}
	te := mustTradeError(t, newSvc(repo).Trade(context.Background(), "w1", "UNKNOWN", "buy"))
	assertCode(t, te, 404)
}

func TestTrade_InsufficientBank_Returns400(t *testing.T) {
	repo := &mockRepo{buyErr: repository.ErrInsufficientBank}
	te := mustTradeError(t, newSvc(repo).Trade(context.Background(), "w1", "AAPL", "buy"))
	assertCode(t, te, 400)
}

func TestTrade_InsufficientWallet_Returns400(t *testing.T) {
	repo := &mockRepo{sellErr: repository.ErrInsufficientWallet}
	te := mustTradeError(t, newSvc(repo).Trade(context.Background(), "w1", "AAPL", "sell"))
	assertCode(t, te, 400)
}

func TestTrade_UnknownRepoError_Returns500(t *testing.T) {
	repo := &mockRepo{buyErr: errors.New("unexpected redis error")}
	te := mustTradeError(t, newSvc(repo).Trade(context.Background(), "w1", "AAPL", "buy"))
	assertCode(t, te, 500)
}

func TestTrade_FailedOperation_DoesNotLog(t *testing.T) {
	repo := &mockRepo{buyErr: repository.ErrInsufficientBank}
	_ = newSvc(repo).Trade(context.Background(), "w1", "AAPL", "buy")

	if len(repo.appendedLogs) != 0 {
		t.Errorf("expected no log entries on failed trade, got %d", len(repo.appendedLogs))
	}
}

// --- GetWallet ---

func TestGetWallet_ReturnsCorrectIDAndStocks(t *testing.T) {
	repo := &mockRepo{walletStocks: []model.StockEntry{{Name: "AAPL", Quantity: 3}}}
	resp, err := newSvc(repo).GetWallet(context.Background(), "wallet1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "wallet1" {
		t.Errorf("expected ID 'wallet1', got '%s'", resp.ID)
	}
	if len(resp.Stocks) != 1 || resp.Stocks[0].Name != "AAPL" {
		t.Errorf("unexpected stocks: %v", resp.Stocks)
	}
}

func TestGetWallet_NilFromRepo_ReturnsEmptySlice(t *testing.T) {
	resp, err := newSvc(&mockRepo{walletStocks: nil}).GetWallet(context.Background(), "wallet1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Stocks == nil {
		t.Error("expected empty slice, got nil — would break JSON clients expecting []")
	}
}

func TestGetWallet_RepoError_ReturnsError(t *testing.T) {
	repo := &mockRepo{walletErr: errors.New("redis down")}
	_, err := newSvc(repo).GetWallet(context.Background(), "wallet1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- GetWalletStock ---

func TestGetWalletStock_ReturnsQuantity(t *testing.T) {
	repo := &mockRepo{walletQty: 5, stockExists: true}
	qty, tradeErr := newSvc(repo).GetWalletStock(context.Background(), "wallet1", "AAPL")

	if tradeErr != nil {
		t.Fatalf("unexpected error: %v", tradeErr)
	}
	if qty != 5 {
		t.Errorf("expected 5, got %d", qty)
	}
}

func TestGetWalletStock_ZeroQuantity_StillReturnsOK(t *testing.T) {
	// Stock exists in bank but wallet holds none — valid state, should return 0 not 404.
	repo := &mockRepo{walletQty: 0, stockExists: true}
	qty, tradeErr := newSvc(repo).GetWalletStock(context.Background(), "wallet1", "AAPL")

	if tradeErr != nil {
		t.Fatalf("unexpected error: %v", tradeErr)
	}
	if qty != 0 {
		t.Errorf("expected 0, got %d", qty)
	}
}

func TestGetWalletStock_StockNotInBank_Returns404(t *testing.T) {
	repo := &mockRepo{stockExists: false}
	_, tradeErr := newSvc(repo).GetWalletStock(context.Background(), "wallet1", "UNKNOWN")

	if tradeErr == nil {
		t.Fatal("expected error, got nil")
	}
	assertCode(t, tradeErr, 404)
}

func TestGetWalletStock_RepoError_Returns500(t *testing.T) {
	repo := &mockRepo{walletQtyErr: errors.New("redis down")}
	_, tradeErr := newSvc(repo).GetWalletStock(context.Background(), "wallet1", "AAPL")

	if tradeErr == nil {
		t.Fatal("expected error, got nil")
	}
	assertCode(t, tradeErr, 500)
}

// --- GetBankStocks ---

func TestGetBankStocks_ReturnsStocks(t *testing.T) {
	repo := &mockRepo{bankStocks: []model.StockEntry{
		{Name: "AAPL", Quantity: 100},
		{Name: "GOOG", Quantity: 50},
	}}
	resp, err := newSvc(repo).GetBankStocks(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Stocks) != 2 {
		t.Errorf("expected 2 stocks, got %d", len(resp.Stocks))
	}
}

func TestGetBankStocks_NilFromRepo_ReturnsEmptySlice(t *testing.T) {
	resp, err := newSvc(&mockRepo{}).GetBankStocks(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Stocks == nil {
		t.Error("expected empty slice, got nil")
	}
}

func TestGetBankStocks_RepoError_ReturnsError(t *testing.T) {
	repo := &mockRepo{bankErr: errors.New("redis down")}
	_, err := newSvc(repo).GetBankStocks(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- SetBankStocks ---

func TestSetBankStocks_SetsCorrectly(t *testing.T) {
	repo := &mockRepo{}
	stocks := []model.StockEntry{{Name: "AAPL", Quantity: 99}}
	err := newSvc(repo).SetBankStocks(context.Background(), stocks)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.bankStocks) != 1 || repo.bankStocks[0].Name != "AAPL" {
		t.Errorf("unexpected bank state: %v", repo.bankStocks)
	}
}

func TestSetBankStocks_EmptyList_ClearsBank(t *testing.T) {
	// Clearing the bank with an empty list is a valid operation.
	repo := &mockRepo{bankStocks: []model.StockEntry{{Name: "AAPL", Quantity: 10}}}
	err := newSvc(repo).SetBankStocks(context.Background(), []model.StockEntry{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.bankStocks) != 0 {
		t.Errorf("expected bank to be empty, got: %v", repo.bankStocks)
	}
}

// --- GetLog ---

func TestGetLog_ReturnsEntriesInOrder(t *testing.T) {
	repo := &mockRepo{logEntries: []model.LogEntry{
		{Type: "buy", WalletID: "w1", StockName: "AAPL"},
		{Type: "sell", WalletID: "w1", StockName: "AAPL"},
	}}
	resp, err := newSvc(repo).GetLog(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Log) != 2 {
		t.Errorf("expected 2 entries, got %d", len(resp.Log))
	}
	if resp.Log[0].Type != "buy" || resp.Log[1].Type != "sell" {
		t.Errorf("unexpected log order: %+v", resp.Log)
	}
}

func TestGetLog_NilFromRepo_ReturnsEmptySlice(t *testing.T) {
	resp, err := newSvc(&mockRepo{}).GetLog(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Log == nil {
		t.Error("expected empty slice, got nil")
	}
}

func TestGetLog_RepoError_ReturnsError(t *testing.T) {
	repo := &mockRepo{logErr: errors.New("redis down")}
	_, err := newSvc(repo).GetLog(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- helpers ---

func mustTradeError(t *testing.T, err error) *service.TradeError {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	te, ok := err.(*service.TradeError)
	if !ok {
		t.Fatalf("expected *service.TradeError, got %T: %v", err, err)
	}
	return te
}

func assertCode(t *testing.T, te *service.TradeError, want int) {
	t.Helper()
	if te.Code != want {
		t.Errorf("expected HTTP code %d, got %d (message: %s)", want, te.Code, te.Message)
	}
}