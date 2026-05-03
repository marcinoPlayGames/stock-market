package service

import (
	"context"
	"errors"
	"stock-market/internal/model"
	"stock-market/internal/repository"
)

type StockService struct {
	repo *repository.RedisRepository
}

func NewStockService(repo *repository.RedisRepository) *StockService {
	return &StockService{repo: repo}
}

type TradeError struct {
	Code    int
	Message string
}

func (e *TradeError) Error() string { return e.Message }

func (s *StockService) Trade(ctx context.Context, walletID, stockName, tradeType string) error {
	var err error
	switch tradeType {
	case "buy":
		err = s.repo.Buy(ctx, walletID, stockName)
	case "sell":
		err = s.repo.Sell(ctx, walletID, stockName)
	default:
		return &TradeError{Code: 400, Message: "invalid trade type, must be 'buy' or 'sell'"}
	}

	if err != nil {
		switch {
		case errors.Is(err, repository.ErrStockNotFound):
			return &TradeError{Code: 404, Message: "stock not found"}
		case errors.Is(err, repository.ErrInsufficientBank):
			return &TradeError{Code: 400, Message: "no stock available in bank"}
		case errors.Is(err, repository.ErrInsufficientWallet):
			return &TradeError{Code: 400, Message: "no stock available in wallet"}
		}
		return &TradeError{Code: 500, Message: "internal error"}
	}

	_ = s.repo.AppendLog(ctx, model.LogEntry{
		Type:      tradeType,
		WalletID:  walletID,
		StockName: stockName,
	})
	return nil
}

func (s *StockService) GetWallet(ctx context.Context, walletID string) (*model.WalletResponse, error) {
	stocks, err := s.repo.GetWallet(ctx, walletID)
	if err != nil {
		return nil, err
	}
	if stocks == nil {
		stocks = []model.StockEntry{}
	}
	return &model.WalletResponse{ID: walletID, Stocks: stocks}, nil
}

func (s *StockService) GetWalletStock(ctx context.Context, walletID, stockName string) (int64, *TradeError) {
	qty, found, err := s.repo.GetWalletStock(ctx, walletID, stockName)
	if err != nil {
		return 0, &TradeError{Code: 500, Message: "internal error"}
	}
	if !found {
		return 0, &TradeError{Code: 404, Message: "stock not found"}
	}
	return qty, nil
}

func (s *StockService) GetBankStocks(ctx context.Context) (*model.BankResponse, error) {
	stocks, err := s.repo.GetBankStocks(ctx)
	if err != nil {
		return nil, err
	}
	if stocks == nil {
		stocks = []model.StockEntry{}
	}
	return &model.BankResponse{Stocks: stocks}, nil
}

func (s *StockService) SetBankStocks(ctx context.Context, stocks []model.StockEntry) error {
	return s.repo.SetBankStocks(ctx, stocks)
}

func (s *StockService) GetLog(ctx context.Context) (*model.LogResponse, error) {
	entries, err := s.repo.GetLog(ctx)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []model.LogEntry{}
	}
	return &model.LogResponse{Log: entries}, nil
}