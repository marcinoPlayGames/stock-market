package repository_test

import (
	"context"
	"encoding/json"
	"testing"

	"stock-market/internal/model"
	"stock-market/internal/repository"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestRepo spins up an in-process miniredis server and returns a
// RedisRepository wired to it. Each test gets a fresh server so there
// is no shared state between cases.
func newTestRepo(t *testing.T) (*repository.RedisRepository, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	return repository.NewRedisRepository(rdb), mr
}

func ctx() context.Context { return context.Background() }

// seedBank sets up stock quantities directly in miniredis so individual
// tests don't have to go through SetBankStocks.
func seedBank(t *testing.T, mr *miniredis.Miniredis, stocks map[string]string) {
	t.Helper()
	for name, qty := range stocks {
		mr.HSet("bank:stocks", name, qty)
	}
}

// --- Buy ---

func TestBuy_Success_DecreasesBankIncreasesWallet(t *testing.T) {
	repo, mr := newTestRepo(t)
	seedBank(t, mr, map[string]string{"AAPL": "5"})

	if err := repo.Buy(ctx(), "wallet1", "AAPL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bankQty := mr.HGet("bank:stocks", "AAPL")
	if bankQty != "4" {
		t.Errorf("expected bank AAPL=4, got %s", bankQty)
	}
	walletQty := mr.HGet("wallet:wallet1:stocks", "AAPL")
	if walletQty != "1" {
		t.Errorf("expected wallet AAPL=1, got %s", walletQty)
	}
}

func TestBuy_StockNotInBank_ReturnsErrStockNotFound(t *testing.T) {
	repo, _ := newTestRepo(t)

	err := repo.Buy(ctx(), "wallet1", "UNKNOWN")
	if err != repository.ErrStockNotFound {
		t.Errorf("expected ErrStockNotFound, got %v", err)
	}
}

func TestBuy_BankOutOfStock_ReturnsErrInsufficientBank(t *testing.T) {
	repo, mr := newTestRepo(t)
	seedBank(t, mr, map[string]string{"AAPL": "0"})

	err := repo.Buy(ctx(), "wallet1", "AAPL")
	if err != repository.ErrInsufficientBank {
		t.Errorf("expected ErrInsufficientBank, got %v", err)
	}
}

func TestBuy_MultipleBuys_QuantitiesAccumulate(t *testing.T) {
	repo, mr := newTestRepo(t)
	seedBank(t, mr, map[string]string{"AAPL": "10"})

	for i := 0; i < 3; i++ {
		if err := repo.Buy(ctx(), "wallet1", "AAPL"); err != nil {
			t.Fatalf("buy %d failed: %v", i+1, err)
		}
	}

	if got := mr.HGet("bank:stocks", "AAPL"); got != "7" {
		t.Errorf("expected bank AAPL=7, got %s", got)
	}
	if got := mr.HGet("wallet:wallet1:stocks", "AAPL"); got != "3" {
		t.Errorf("expected wallet AAPL=3, got %s", got)
	}
}

// --- Sell ---

func TestSell_Success_IncreasesBankDecreasesWallet(t *testing.T) {
	repo, mr := newTestRepo(t)
	seedBank(t, mr, map[string]string{"AAPL": "4"})
	mr.HSet("wallet:wallet1:stocks", "AAPL", "2")

	if err := repo.Sell(ctx(), "wallet1", "AAPL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := mr.HGet("bank:stocks", "AAPL"); got != "5" {
		t.Errorf("expected bank AAPL=5, got %s", got)
	}
	if got := mr.HGet("wallet:wallet1:stocks", "AAPL"); got != "1" {
		t.Errorf("expected wallet AAPL=1, got %s", got)
	}
}

func TestSell_StockNotInBank_ReturnsErrStockNotFound(t *testing.T) {
	repo, _ := newTestRepo(t)

	err := repo.Sell(ctx(), "wallet1", "UNKNOWN")
	if err != repository.ErrStockNotFound {
		t.Errorf("expected ErrStockNotFound, got %v", err)
	}
}

func TestSell_WalletEmpty_ReturnsErrInsufficientWallet(t *testing.T) {
	repo, mr := newTestRepo(t)
	seedBank(t, mr, map[string]string{"AAPL": "10"})
	// wallet has no AAPL

	err := repo.Sell(ctx(), "wallet1", "AAPL")
	if err != repository.ErrInsufficientWallet {
		t.Errorf("expected ErrInsufficientWallet, got %v", err)
	}
}

func TestSell_WalletZeroQuantity_ReturnsErrInsufficientWallet(t *testing.T) {
	repo, mr := newTestRepo(t)
	seedBank(t, mr, map[string]string{"AAPL": "10"})
	mr.HSet("wallet:wallet1:stocks", "AAPL", "0")

	err := repo.Sell(ctx(), "wallet1", "AAPL")
	if err != repository.ErrInsufficientWallet {
		t.Errorf("expected ErrInsufficientWallet, got %v", err)
	}
}

// --- GetWallet ---

func TestGetWallet_ReturnsOwnedStocks(t *testing.T) {
	repo, mr := newTestRepo(t)
	mr.HSet("wallet:wallet1:stocks", "AAPL", "3", "GOOG", "1")

	stocks, err := repo.GetWallet(ctx(), "wallet1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stocks) != 2 {
		t.Errorf("expected 2 stocks, got %d: %v", len(stocks), stocks)
	}
}

func TestGetWallet_EmptyWallet_ReturnsEmptySlice(t *testing.T) {
	repo, _ := newTestRepo(t)

	stocks, err := repo.GetWallet(ctx(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stocks) != 0 {
		t.Errorf("expected empty slice, got %v", stocks)
	}
}

// --- GetWalletStock ---

func TestGetWalletStock_StockExistsInBank_ReturnsQuantityAndFound(t *testing.T) {
	repo, mr := newTestRepo(t)
	seedBank(t, mr, map[string]string{"AAPL": "10"})
	mr.HSet("wallet:wallet1:stocks", "AAPL", "4")

	qty, found, err := repo.GetWalletStock(ctx(), "wallet1", "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if qty != 4 {
		t.Errorf("expected 4, got %d", qty)
	}
}

func TestGetWalletStock_StockNotInBank_ReturnsFalse(t *testing.T) {
	repo, _ := newTestRepo(t)

	_, found, err := repo.GetWalletStock(ctx(), "wallet1", "UNKNOWN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected found=false for stock not in bank")
	}
}

func TestGetWalletStock_StockInBankButNotInWallet_ReturnsZeroAndFound(t *testing.T) {
	repo, mr := newTestRepo(t)
	seedBank(t, mr, map[string]string{"AAPL": "10"})
	// wallet does not hold AAPL

	qty, found, err := repo.GetWalletStock(ctx(), "wallet1", "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true — stock exists in bank")
	}
	if qty != 0 {
		t.Errorf("expected qty=0, got %d", qty)
	}
}

// --- GetBankStocks / SetBankStocks ---

func TestSetAndGetBankStocks_RoundTrip(t *testing.T) {
	repo, _ := newTestRepo(t)
	input := []model.StockEntry{
		{Name: "AAPL", Quantity: 100},
		{Name: "GOOG", Quantity: 50},
	}

	if err := repo.SetBankStocks(ctx(), input); err != nil {
		t.Fatalf("SetBankStocks error: %v", err)
	}

	got, err := repo.GetBankStocks(ctx())
	if err != nil {
		t.Fatalf("GetBankStocks error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 stocks, got %d: %v", len(got), got)
	}
}

func TestSetBankStocks_ReplacesExistingStocks(t *testing.T) {
	repo, mr := newTestRepo(t)
	seedBank(t, mr, map[string]string{"AAPL": "100", "GOOG": "50"})

	// Replace with only TSLA — AAPL and GOOG should be gone.
	if err := repo.SetBankStocks(ctx(), []model.StockEntry{{Name: "TSLA", Quantity: 75}}); err != nil {
		t.Fatalf("SetBankStocks error: %v", err)
	}

	got, err := repo.GetBankStocks(ctx())
	if err != nil {
		t.Fatalf("GetBankStocks error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "TSLA" {
		t.Errorf("expected only TSLA, got %v", got)
	}
}

func TestSetBankStocks_EmptyList_ClearsAllStocks(t *testing.T) {
	repo, mr := newTestRepo(t)
	seedBank(t, mr, map[string]string{"AAPL": "100"})

	if err := repo.SetBankStocks(ctx(), []model.StockEntry{}); err != nil {
		t.Fatalf("SetBankStocks error: %v", err)
	}

	got, err := repo.GetBankStocks(ctx())
	if err != nil {
		t.Fatalf("GetBankStocks error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty bank, got %v", got)
	}
}

// --- AppendLog / GetLog ---

func TestAppendAndGetLog_PreservesInsertionOrder(t *testing.T) {
	repo, _ := newTestRepo(t)

	entries := []model.LogEntry{
		{Type: "buy", WalletID: "w1", StockName: "AAPL"},
		{Type: "sell", WalletID: "w1", StockName: "AAPL"},
		{Type: "buy", WalletID: "w2", StockName: "GOOG"},
	}
	for _, e := range entries {
		if err := repo.AppendLog(ctx(), e); err != nil {
			t.Fatalf("AppendLog error: %v", err)
		}
	}

	got, err := repo.GetLog(ctx())
	if err != nil {
		t.Fatalf("GetLog error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	for i, want := range entries {
		if got[i] != want {
			t.Errorf("entry %d: expected %+v, got %+v", i, want, got[i])
		}
	}
}

func TestGetLog_Empty_ReturnsEmptySlice(t *testing.T) {
	repo, _ := newTestRepo(t)

	got, err := repo.GetLog(ctx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty log, got %v", got)
	}
}

func TestAppendLog_MalformedEntryInRedis_IsSkipped(t *testing.T) {
	// If someone manually corrupts the log list, GetLog should skip bad
	// entries rather than returning an error or panicking.
	repo, mr := newTestRepo(t)
	mr.RPush("audit:log", "not-valid-json")

	valid, _ := json.Marshal(model.LogEntry{Type: "buy", WalletID: "w1", StockName: "AAPL"})
	mr.RPush("audit:log", string(valid))

	got, err := repo.GetLog(ctx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 valid entry (bad one skipped), got %d: %v", len(got), got)
	}
}