package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"stock-market/internal/model"
	"stock-market/internal/service"

	"github.com/go-chi/chi/v5"
	"log"
	"fmt"
	"time"
)

type Handler struct {
	svc *service.StockService
}

func NewHandler(svc *service.StockService) *Handler {
	return &Handler{svc: svc}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (h *Handler) TradeStock(w http.ResponseWriter, r *http.Request) {
	walletID := chi.URLParam(r, "wallet_id")
	stockName := chi.URLParam(r, "stock_name")

	var req model.TradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.svc.Trade(r.Context(), walletID, stockName, req.Type); err != nil {
		if te, ok := err.(*service.TradeError); ok {
			writeError(w, te.Code, te.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) GetWallet(w http.ResponseWriter, r *http.Request) {
	walletID := chi.URLParam(r, "wallet_id")
	resp, err := h.svc.GetWallet(r.Context(), walletID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetWalletStock(w http.ResponseWriter, r *http.Request) {
	walletID := chi.URLParam(r, "wallet_id")
	stockName := chi.URLParam(r, "stock_name")
	qty, tradeErr := h.svc.GetWalletStock(r.Context(), walletID, stockName)
	if tradeErr != nil {
		writeError(w, tradeErr.Code, tradeErr.Message)
		return
	}
	writeJSON(w, http.StatusOK, qty)
}

func (h *Handler) GetBankStocks(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.GetBankStocks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) SetBankStocks(w http.ResponseWriter, r *http.Request) {
	var req model.SetBankRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.svc.SetBankStocks(r.Context(), req.Stocks); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) GetLog(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.GetLog(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) Chaos(w http.ResponseWriter, r *http.Request) {
	log.Printf("Chaos triggered on instance")

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, "killing instance...\n")

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.Exit(1)
	}()
}