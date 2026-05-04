package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"stock-market/internal/model"

	"github.com/redis/go-redis/v9"
	"strconv"
)

var (
	ErrStockNotFound      = errors.New("stock not found in bank")
	ErrInsufficientBank   = errors.New("insufficient stock in bank")
	ErrInsufficientWallet = errors.New("insufficient stock in wallet")
)

const (
	bankKey = "bank:stocks"
	logKey  = "audit:log"
)

func walletKey(walletID string) string {
	return fmt.Sprintf("wallet:%s:stocks", walletID)
}

// Repository is the interface that both the real Redis implementation
// and test mocks satisfy.
type Repository interface {
	Buy(ctx context.Context, walletID, stockName string) error
	Sell(ctx context.Context, walletID, stockName string) error
	GetWallet(ctx context.Context, walletID string) ([]model.StockEntry, error)
	GetWalletStock(ctx context.Context, walletID, stockName string) (int64, bool, error)
	GetBankStocks(ctx context.Context) ([]model.StockEntry, error)
	SetBankStocks(ctx context.Context, stocks []model.StockEntry) error
	AppendLog(ctx context.Context, entry model.LogEntry) error
	GetLog(ctx context.Context) ([]model.LogEntry, error)
}

// RedisRepository is the production implementation of Repository.
type RedisRepository struct {
	rdb *redis.Client
}

func NewRedisRepository(rdb *redis.Client) *RedisRepository {
	return &RedisRepository{rdb: rdb}
}

var buyScript = redis.NewScript(`
local bankKey = KEYS[1]
local walletKey = KEYS[2]
local stock = ARGV[1]

local bankQty = tonumber(redis.call("HGET", bankKey, stock))
if bankQty == nil then
    return -1
end
if bankQty <= 0 then
    return -2
end
redis.call("HINCRBY", bankKey, stock, -1)
redis.call("HINCRBY", walletKey, stock, 1)
return 1
`)

var sellScript = redis.NewScript(`
local bankKey = KEYS[1]
local walletKey = KEYS[2]
local stock = ARGV[1]

local bankExists = redis.call("HEXISTS", bankKey, stock)
if bankExists == 0 then
    return -1
end

local walletQty = tonumber(redis.call("HGET", walletKey, stock))
if walletQty == nil or walletQty <= 0 then
    return -3
end

redis.call("HINCRBY", bankKey, stock, 1)
redis.call("HINCRBY", walletKey, stock, -1)
return 1
`)

func (r *RedisRepository) Buy(ctx context.Context, walletID, stockName string) error {
	result, err := buyScript.Run(ctx, r.rdb,
		[]string{bankKey, walletKey(walletID)},
		stockName,
	).Int()
	if err != nil {
		return err
	}
	switch result {
	case -1:
		return ErrStockNotFound
	case -2:
		return ErrInsufficientBank
	}
	return nil
}

func (r *RedisRepository) Sell(ctx context.Context, walletID, stockName string) error {
	result, err := sellScript.Run(ctx, r.rdb,
		[]string{bankKey, walletKey(walletID)},
		stockName,
	).Int()
	if err != nil {
		return err
	}
	switch result {
	case -1:
		return ErrStockNotFound
	case -3:
		return ErrInsufficientWallet
	}
	return nil
}

func (r *RedisRepository) GetWallet(ctx context.Context, walletID string) ([]model.StockEntry, error) {
	data, err := r.rdb.HGetAll(ctx, walletKey(walletID)).Result()
	if err != nil {
		return nil, fmt.Errorf("get wallet failed: %w", err)
	}
	return parseStockMap(data), nil
}

func (r *RedisRepository) GetWalletStock(ctx context.Context, walletID, stockName string) (int64, bool, error) {
	exists, err := r.rdb.HExists(ctx, bankKey, stockName).Result()
	if err != nil {
		return 0, false, err
	}
	if !exists {
		return 0, false, nil
	}
	val, err := r.rdb.HGet(ctx, walletKey(walletID), stockName).Int64()
	if err == redis.Nil {
		return 0, true, nil
	}
	return val, true, err
}

func (r *RedisRepository) GetBankStocks(ctx context.Context) ([]model.StockEntry, error) {
	data, err := r.rdb.HGetAll(ctx, bankKey).Result()
	if err != nil {
		return nil, fmt.Errorf("get bank stocks failed: %w", err)
	}
	return parseStockMap(data), nil
}

func (r *RedisRepository) SetBankStocks(ctx context.Context, stocks []model.StockEntry) error {
	pipe := r.rdb.TxPipeline()
	pipe.Del(ctx, bankKey)
	if len(stocks) > 0 {
		fields := make(map[string]interface{}, len(stocks))
		for _, s := range stocks {
			fields[s.Name] = s.Quantity
		}
		pipe.HSet(ctx, bankKey, fields)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("set bank stocks failed: %w", err)
	}
	return err
}

func (r *RedisRepository) AppendLog(ctx context.Context, entry model.LogEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return r.rdb.RPush(ctx, logKey, data).Err()
}

func (r *RedisRepository) GetLog(ctx context.Context) ([]model.LogEntry, error) {
	items, err := r.rdb.LRange(ctx, logKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("get log failed: %w", err)
	}
	entries := make([]model.LogEntry, 0, len(items))
	for _, item := range items {
		var e model.LogEntry
		if err := json.Unmarshal([]byte(item), &e); err == nil {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

func parseStockMap(data map[string]string) []model.StockEntry {
	entries := make([]model.StockEntry, 0, len(data))
	for name, qtyStr := range data {
		qty, err := strconv.ParseInt(qtyStr, 10, 64)
		if err != nil {
			// Opcjonalnie: log.Printf("Error parsing quantity for %s: %v", name, err)
			continue // Skipping broken record
		}
		entries = append(entries, model.StockEntry{Name: name, Quantity: qty})
	}
	return entries
}