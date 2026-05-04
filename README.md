# Stock Market Service

A simplified stock market simulation service built in Go, designed for high availability and horizontal scalability.

## Architecture

```
Client → Nginx (Load Balancer) → App Instance 1 ─┐
                               → App Instance 2 ─┴→ Redis (Shared State)
```

The service runs as a multi-container Docker application:

- **Nginx** — load balancer distributing traffic across application instances
- **App (x2)** — Go HTTP servers handling all business logic
- **Redis** — shared state store ensuring consistency across instances (atomic Lua scripts for buy/sell operations)

This design ensures that killing any single application instance (e.g. via `POST /chaos`) does not take down the service, as the remaining instance continues to serve traffic. Docker's `restart: always` policy automatically restores the killed instance.

## Prerequisites

- Docker Desktop (tested on 4.33.0+)
- Docker Compose v2

No other dependencies are required — Go runtime is only needed if you wish to modify the source code.

## Running the Service

```bash
chmod +x start.sh
./start.sh <PORT>
```

Example:

```bash
./start.sh 3000
```

The service will be available at `http://localhost:3000`.

On Windows, run via Git Bash or WSL:

```bash
sh start.sh 3000
```

## API Reference

### Wallets

#### `POST /wallets/{wallet_id}/stocks/{stock_name}`

Buy or sell a single unit of a stock.

Request body:
```json
{ "type": "buy" }
```
or
```json
{ "type": "sell" }
```

| Condition | Response |
|---|---|
| Success | `200 OK` |
| Stock does not exist | `404 Not Found` |
| Buy — no stock available in bank | `400 Bad Request` |
| Sell — no stock in wallet | `400 Bad Request` |

If the wallet does not exist, it is created automatically on first operation.

---

#### `GET /wallets/{wallet_id}`

Returns the current state of a wallet.

Response:
```json
{
  "id": "wallet1",
  "stocks": [
    { "name": "AAPL", "quantity": 5 },
    { "name": "GOOG", "quantity": 2 }
  ]
}
```

---

#### `GET /wallets/{wallet_id}/stocks/{stock_name}`

Returns the quantity of a specific stock in a wallet.

Response:
```json
5
```

Returns `404` if the stock does not exist in the bank.

---

### Bank

#### `GET /stocks`

Returns the current state of the bank.

Response:
```json
{
  "stocks": [
    { "name": "AAPL", "quantity": 95 },
    { "name": "GOOG", "quantity": 100 }
  ]
}
```

---

#### `POST /stocks`

Sets the state of the bank, replacing all existing stock quantities.

Request body:
```json
{
  "stocks": [
    { "name": "AAPL", "quantity": 100 },
    { "name": "GOOG", "quantity": 100 }
  ]
}
```

Response: `200 OK`

---

### Audit Log

#### `GET /log`

Returns all successful buy/sell operations in order of occurrence. Bank operations are excluded.

Response:
```json
{
  "log": [
    { "type": "buy",  "wallet_id": "wallet1", "stock_name": "AAPL" },
    { "type": "sell", "wallet_id": "wallet1", "stock_name": "AAPL" }
  ]
}
```

---

### Chaos

#### `POST /chaos`

Kills the instance that serves this request. The service remains available via the remaining instance. The killed instance is automatically restarted by Docker.

---

## Example Walkthrough

```bash
# 1. Seed the bank with stocks
curl -X POST http://localhost:3000/stocks \
  -H "Content-Type: application/json" \
  -d '{"stocks":[{"name":"AAPL","quantity":100},{"name":"GOOG","quantity":50}]}'

# 2. Buy a stock (wallet is created automatically)
curl -X POST http://localhost:3000/wallets/wallet1/stocks/AAPL \
  -H "Content-Type: application/json" \
  -d '{"type":"buy"}'

# 3. Check wallet state
curl http://localhost:3000/wallets/wallet1

# 4. Check bank state (AAPL quantity decreased by 1)
curl http://localhost:3000/stocks

# 5. Sell the stock back
curl -X POST http://localhost:3000/wallets/wallet1/stocks/AAPL \
  -H "Content-Type: application/json" \
  -d '{"type":"sell"}'

# 6. Check the audit log
curl http://localhost:3000/log

# 7. Test chaos — service remains available
curl -X POST http://localhost:3000/chaos
curl http://localhost:3000/stocks  # still responds
```

## Design Decisions

### Atomicity of buy/sell operations

Buy and sell operations are implemented as Redis Lua scripts, which execute atomically on the Redis server. This eliminates race conditions when multiple requests arrive concurrently — no distributed locks are needed.

### High Availability

Two application instances run behind Nginx. If one is killed (via `/chaos` or any other reason), Nginx automatically routes all traffic to the surviving instance. Docker's `restart: always` policy brings the killed instance back within seconds.

### Shared state via Redis

All application state (bank stocks, wallet stocks, audit log) is stored in Redis, not in application memory. This means any instance can handle any request — there is no session affinity required.

### Audit log ordering

The audit log is stored as a Redis List using `RPUSH`, which guarantees insertion order. `LRANGE 0 -1` retrieves all entries in the correct sequence.

### Cross-platform compatibility

The entire runtime is containerised. The only requirement on the host machine is Docker — no Go installation, no OS-specific tooling. The multi-stage Dockerfile produces a minimal Alpine-based image that runs identically on Linux, macOS, and Windows, on both x64 and arm64 architectures.

## Project Structure

```
stock-market/
├── cmd/
│   └── server/
│       └── main.go           # Entry point, router setup
├── internal/
│   ├── handler/
│   │   └── handler.go        # HTTP request handlers
│   ├── service/
│   │   └── stock_service.go  # Business logic
│   ├── repository/
│   │   └── redis.go          # Redis operations, Lua scripts
│   └── model/
│       └── model.go          # Request/response structs
├── Dockerfile                # Multi-stage build
├── docker-compose.yml        # Orchestration (nginx + 2x app + redis)
├── nginx.conf                # Load balancer config
├── start.sh                  # Single-command startup script
├── go.mod
├── go.sum
└── README.md
```

## Stopping the Service

```bash
docker compose down
```
