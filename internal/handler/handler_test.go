package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"stock-market/internal/handler"
	"stock-market/internal/model"
	"stock-market/internal/repository"
	"stock-market/internal/service"

	"github.com/go-chi/chi/v5"
)

// --- Mock Repository for handler tests ---

type mockRepo struct {
	buyErr       error
	sellErr      error
	walletStocks []model.StockEntry
	walletQty    int64
	stockExists  bool
	bankStocks   []model.StockEntry
	logEntries   []model.LogEntry
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

func (m *mockRepo) AppendLog(_ context.Context, _ model.LogEntry) error { return nil }

func (m *mockRepo) GetLog(_ context.Context) ([]model.LogEntry, error) {
	return m.logEntries, nil
}

// --- Helpers ---

func newRouter(h *handler.Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Post("/wallets/{wallet_id}/stocks/{stock_name}", h.TradeStock)
	r.Get("/wallets/{wallet_id}", h.GetWallet)
	r.Get("/wallets/{wallet_id}/stocks/{stock_name}", h.GetWalletStock)
	r.Get("/stocks", h.GetBankStocks)
	r.Post("/stocks", h.SetBankStocks)
	r.Get("/log", h.GetLog)
	return r
}

func newHandler(repo *mockRepo) (*handler.Handler, *chi.Mux) {
	svc := service.NewStockService(repo)
	h := handler.NewHandler(svc)
	return h, newRouter(h)
}

// --- TradeStock Tests ---

func TestTradeStock_Buy_Returns200(t *testing.T) {
	_, r := newHandler(&mockRepo{})

	body := `{"type":"buy"}`
	req := httptest.NewRequest(http.MethodPost, "/wallets/wallet1/stocks/AAPL", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestTradeStock_Sell_Returns200(t *testing.T) {
	_, r := newHandler(&mockRepo{})

	body := `{"type":"sell"}`
	req := httptest.NewRequest(http.MethodPost, "/wallets/wallet1/stocks/AAPL", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestTradeStock_InvalidBody_Returns400(t *testing.T) {
	_, r := newHandler(&mockRepo{})

	req := httptest.NewRequest(http.MethodPost, "/wallets/wallet1/stocks/AAPL", bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTradeStock_StockNotFound_Returns404(t *testing.T) {
	_, r := newHandler(&mockRepo{buyErr: repository.ErrStockNotFound})

	body := `{"type":"buy"}`
	req := httptest.NewRequest(http.MethodPost, "/wallets/wallet1/stocks/UNKNOWN", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTradeStock_InsufficientBank_Returns400(t *testing.T) {
	_, r := newHandler(&mockRepo{buyErr: repository.ErrInsufficientBank})

	body := `{"type":"buy"}`
	req := httptest.NewRequest(http.MethodPost, "/wallets/wallet1/stocks/AAPL", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTradeStock_InsufficientWallet_Returns400(t *testing.T) {
	_, r := newHandler(&mockRepo{sellErr: repository.ErrInsufficientWallet})

	body := `{"type":"sell"}`
	req := httptest.NewRequest(http.MethodPost, "/wallets/wallet1/stocks/AAPL", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- GetWallet Tests ---

func TestGetWallet_Returns200WithStocks(t *testing.T) {
	repo := &mockRepo{
		walletStocks: []model.StockEntry{
			{Name: "AAPL", Quantity: 3},
		},
	}
	_, r := newHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/wallets/wallet1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp model.WalletResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.ID != "wallet1" {
		t.Errorf("expected ID 'wallet1', got '%s'", resp.ID)
	}
	if len(resp.Stocks) != 1 || resp.Stocks[0].Name != "AAPL" {
		t.Errorf("unexpected stocks: %v", resp.Stocks)
	}
}

func TestGetWallet_EmptyWallet_ReturnsEmptyStocks(t *testing.T) {
	_, r := newHandler(&mockRepo{})

	req := httptest.NewRequest(http.MethodGet, "/wallets/wallet1", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp model.WalletResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Stocks == nil {
		t.Error("expected empty slice, got nil")
	}
}

// --- GetWalletStock Tests ---

func TestGetWalletStock_Returns200WithQuantity(t *testing.T) {
	repo := &mockRepo{walletQty: 7, stockExists: true}
	_, r := newHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/wallets/wallet1/stocks/AAPL", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var qty int64
	if err := json.NewDecoder(w.Body).Decode(&qty); err != nil {
		t.Fatalf("failed to decode quantity: %v", err)
	}
	if qty != 7 {
		t.Errorf("expected quantity 7, got %d", qty)
	}
}

func TestGetWalletStock_StockNotFound_Returns404(t *testing.T) {
	repo := &mockRepo{stockExists: false}
	_, r := newHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/wallets/wallet1/stocks/UNKNOWN", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- GetBankStocks Tests ---

func TestGetBankStocks_Returns200(t *testing.T) {
	repo := &mockRepo{
		bankStocks: []model.StockEntry{
			{Name: "AAPL", Quantity: 100},
		},
	}
	_, r := newHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/stocks", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp model.BankResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Stocks) != 1 {
		t.Errorf("expected 1 stock, got %d", len(resp.Stocks))
	}
}

// --- SetBankStocks Tests ---

func TestSetBankStocks_Returns200(t *testing.T) {
	_, r := newHandler(&mockRepo{})

	body := `{"stocks":[{"name":"AAPL","quantity":100}]}`
	req := httptest.NewRequest(http.MethodPost, "/stocks", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSetBankStocks_InvalidBody_Returns400(t *testing.T) {
	_, r := newHandler(&mockRepo{})

	req := httptest.NewRequest(http.MethodPost, "/stocks", bytes.NewBufferString("bad json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- GetLog Tests ---

func TestGetLog_Returns200WithEntries(t *testing.T) {
	repo := &mockRepo{
		logEntries: []model.LogEntry{
			{Type: "buy", WalletID: "w1", StockName: "AAPL"},
		},
	}
	_, r := newHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/log", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp model.LogResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Log) != 1 {
		t.Errorf("expected 1 log entry, got %d", len(resp.Log))
	}
}

func TestGetLog_Empty_ReturnsEmptySlice(t *testing.T) {
	_, r := newHandler(&mockRepo{})

	req := httptest.NewRequest(http.MethodGet, "/log", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp model.LogResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Log == nil {
		t.Error("expected empty slice, got nil")
	}
}