package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"stock-market/internal/handler"
	"stock-market/internal/model"
	"stock-market/internal/repository"
	"stock-market/internal/service"

	"github.com/go-chi/chi/v5"
)

// mockRepo implements repository.Repository. Because NewStockService now
// accepts the interface, the mock injects cleanly with no real Redis needed.
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

func (m *mockRepo) AppendLog(_ context.Context, _ model.LogEntry) error { return nil }

func (m *mockRepo) GetLog(_ context.Context) ([]model.LogEntry, error) {
	return m.logEntries, m.logErr
}

// compile-time guard
var _ repository.Repository = (*mockRepo)(nil)

// helpers

func newRouter(repo *mockRepo) *chi.Mux {
	svc := service.NewStockService(repo)
	h := handler.NewHandler(svc)

	r := chi.NewRouter()
	r.Post("/wallets/{wallet_id}/stocks/{stock_name}", h.TradeStock)
	r.Get("/wallets/{wallet_id}", h.GetWallet)
	r.Get("/wallets/{wallet_id}/stocks/{stock_name}", h.GetWalletStock)
	r.Get("/stocks", h.GetBankStocks)
	r.Post("/stocks", h.SetBankStocks)
	r.Get("/log", h.GetLog)
	return r
}

func doRequest(t *testing.T, r *chi.Mux, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Errorf("expected status %d, got %d (body: %s)", want, w.Code, w.Body.String())
	}
}

// --- TradeStock ---

func TestTradeStock_Buy_Returns200(t *testing.T) {
	w := doRequest(t, newRouter(&mockRepo{}), http.MethodPost, "/wallets/wallet1/stocks/AAPL", `{"type":"buy"}`)
	assertStatus(t, w, http.StatusOK)
}

func TestTradeStock_Sell_Returns200(t *testing.T) {
	w := doRequest(t, newRouter(&mockRepo{}), http.MethodPost, "/wallets/wallet1/stocks/AAPL", `{"type":"sell"}`)
	assertStatus(t, w, http.StatusOK)
}

func TestTradeStock_InvalidJSON_Returns400(t *testing.T) {
	w := doRequest(t, newRouter(&mockRepo{}), http.MethodPost, "/wallets/wallet1/stocks/AAPL", `not-json`)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestTradeStock_InvalidTradeType_Returns400(t *testing.T) {
	w := doRequest(t, newRouter(&mockRepo{}), http.MethodPost, "/wallets/wallet1/stocks/AAPL", `{"type":"hodl"}`)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestTradeStock_StockNotFound_Returns404(t *testing.T) {
	repo := &mockRepo{buyErr: repository.ErrStockNotFound}
	w := doRequest(t, newRouter(repo), http.MethodPost, "/wallets/wallet1/stocks/UNKNOWN", `{"type":"buy"}`)
	assertStatus(t, w, http.StatusNotFound)
}

func TestTradeStock_InsufficientBank_Returns400(t *testing.T) {
	repo := &mockRepo{buyErr: repository.ErrInsufficientBank}
	w := doRequest(t, newRouter(repo), http.MethodPost, "/wallets/wallet1/stocks/AAPL", `{"type":"buy"}`)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestTradeStock_InsufficientWallet_Returns400(t *testing.T) {
	repo := &mockRepo{sellErr: repository.ErrInsufficientWallet}
	w := doRequest(t, newRouter(repo), http.MethodPost, "/wallets/wallet1/stocks/AAPL", `{"type":"sell"}`)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestTradeStock_UnknownRepoError_Returns500(t *testing.T) {
	repo := &mockRepo{buyErr: errors.New("unexpected redis failure")}
	w := doRequest(t, newRouter(repo), http.MethodPost, "/wallets/wallet1/stocks/AAPL", `{"type":"buy"}`)
	assertStatus(t, w, http.StatusInternalServerError)
}

// --- GetWallet ---

func TestGetWallet_Returns200WithStocks(t *testing.T) {
	repo := &mockRepo{walletStocks: []model.StockEntry{{Name: "AAPL", Quantity: 3}}}
	w := doRequest(t, newRouter(repo), http.MethodGet, "/wallets/wallet1", "")
	assertStatus(t, w, http.StatusOK)

	var resp model.WalletResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.ID != "wallet1" {
		t.Errorf("expected ID 'wallet1', got '%s'", resp.ID)
	}
	if len(resp.Stocks) != 1 || resp.Stocks[0].Name != "AAPL" {
		t.Errorf("unexpected stocks: %v", resp.Stocks)
	}
}

func TestGetWallet_EmptyWallet_ReturnsEmptySlice(t *testing.T) {
	w := doRequest(t, newRouter(&mockRepo{}), http.MethodGet, "/wallets/wallet1", "")
	assertStatus(t, w, http.StatusOK)

	var resp model.WalletResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Stocks == nil {
		t.Error("expected empty slice, got nil")
	}
}

func TestGetWallet_RepoError_Returns500(t *testing.T) {
	repo := &mockRepo{walletErr: errors.New("redis down")}
	w := doRequest(t, newRouter(repo), http.MethodGet, "/wallets/wallet1", "")
	assertStatus(t, w, http.StatusInternalServerError)
}

// --- GetWalletStock ---

func TestGetWalletStock_Returns200WithQuantity(t *testing.T) {
	repo := &mockRepo{walletQty: 7, stockExists: true}
	w := doRequest(t, newRouter(repo), http.MethodGet, "/wallets/wallet1/stocks/AAPL", "")
	assertStatus(t, w, http.StatusOK)

	var qty int64
	if err := json.NewDecoder(w.Body).Decode(&qty); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if qty != 7 {
		t.Errorf("expected 7, got %d", qty)
	}
}

func TestGetWalletStock_StockNotInBank_Returns404(t *testing.T) {
	repo := &mockRepo{stockExists: false}
	w := doRequest(t, newRouter(repo), http.MethodGet, "/wallets/wallet1/stocks/UNKNOWN", "")
	assertStatus(t, w, http.StatusNotFound)
}

func TestGetWalletStock_RepoError_Returns500(t *testing.T) {
	repo := &mockRepo{walletQtyErr: errors.New("redis down")}
	w := doRequest(t, newRouter(repo), http.MethodGet, "/wallets/wallet1/stocks/AAPL", "")
	assertStatus(t, w, http.StatusInternalServerError)
}

// --- GetBankStocks ---

func TestGetBankStocks_Returns200WithStocks(t *testing.T) {
	repo := &mockRepo{bankStocks: []model.StockEntry{{Name: "AAPL", Quantity: 100}}}
	w := doRequest(t, newRouter(repo), http.MethodGet, "/stocks", "")
	assertStatus(t, w, http.StatusOK)

	var resp model.BankResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Stocks) != 1 {
		t.Errorf("expected 1 stock, got %d", len(resp.Stocks))
	}
}

func TestGetBankStocks_RepoError_Returns500(t *testing.T) {
	repo := &mockRepo{bankErr: errors.New("redis down")}
	w := doRequest(t, newRouter(repo), http.MethodGet, "/stocks", "")
	assertStatus(t, w, http.StatusInternalServerError)
}

// --- SetBankStocks ---

func TestSetBankStocks_Returns200(t *testing.T) {
	w := doRequest(t, newRouter(&mockRepo{}), http.MethodPost, "/stocks", `{"stocks":[{"name":"AAPL","quantity":100}]}`)
	assertStatus(t, w, http.StatusOK)
}

func TestSetBankStocks_EmptyStocks_Returns200(t *testing.T) {
	// Clearing the bank is a valid operation.
	w := doRequest(t, newRouter(&mockRepo{}), http.MethodPost, "/stocks", `{"stocks":[]}`)
	assertStatus(t, w, http.StatusOK)
}

func TestSetBankStocks_InvalidJSON_Returns400(t *testing.T) {
	w := doRequest(t, newRouter(&mockRepo{}), http.MethodPost, "/stocks", `bad json`)
	assertStatus(t, w, http.StatusBadRequest)
}

// --- GetLog ---

func TestGetLog_Returns200WithEntries(t *testing.T) {
	repo := &mockRepo{logEntries: []model.LogEntry{
		{Type: "buy", WalletID: "w1", StockName: "AAPL"},
	}}
	w := doRequest(t, newRouter(repo), http.MethodGet, "/log", "")
	assertStatus(t, w, http.StatusOK)

	var resp model.LogResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Log) != 1 {
		t.Errorf("expected 1 entry, got %d", len(resp.Log))
	}
}

func TestGetLog_Empty_ReturnsEmptySlice(t *testing.T) {
	w := doRequest(t, newRouter(&mockRepo{}), http.MethodGet, "/log", "")
	assertStatus(t, w, http.StatusOK)

	var resp model.LogResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Log == nil {
		t.Error("expected empty slice, got nil")
	}
}

func TestGetLog_RepoError_Returns500(t *testing.T) {
	repo := &mockRepo{logErr: errors.New("redis down")}
	w := doRequest(t, newRouter(repo), http.MethodGet, "/log", "")
	assertStatus(t, w, http.StatusInternalServerError)
}