package model

type TradeRequest struct {
	Type string `json:"type"` // "buy" | "sell"
}

type StockEntry struct {
	Name     string `json:"name"`
	Quantity int64  `json:"quantity"`
}

type WalletResponse struct {
	ID     string       `json:"id"`
	Stocks []StockEntry `json:"stocks"`
}

type BankResponse struct {
	Stocks []StockEntry `json:"stocks"`
}

type LogEntry struct {
	Type      string `json:"type"`
	WalletID  string `json:"wallet_id"`
	StockName string `json:"stock_name"`
}

type LogResponse struct {
	Log []LogEntry `json:"log"`
}

type SetBankRequest struct {
	Stocks []StockEntry `json:"stocks"`
}