# Cloud Auction Microservices

Portfolio-grade **cloud-based online auction system** built with Go (Gin), PostgreSQL, Redis, Docker Compose, Kubernetes manifests, and GitHub Actions CI.

## Architecture

```text
Clients (Web/Mobile)
   |
   | JWT (Authorization: Token <JWT>
   v
+--------------------+       internal token        +--------------------+
|   Auction Service  | <-------------------------> |    Bid Service     |
| 8081, REST + cache |                             | 8082, REST + WS    |
+--------------------+                             +--------------------+
          ^                                                  ^
          |                                                  |
          |                                                  |
          +----------------------+---------------------------+
                                 |
                                 v
                       +--------------------+
                       |   User Service     |
                       | 8080, JWT issuer   |
                       +--------------------+

Data layer:
- PostgreSQL: users, auctions, bids
- Redis: auction list/details cache, highest-bid cache
```

## Services

- **User Service** (`cmd/user-service`)
  - `POST /register`
  - `POST /login` (issues JWT)
  - `GET /profile` (auth required)
- **Auction Service** (`cmd/auction-service`)
  - `POST /auctions` (seller only)
  - `GET /auctions`
  - `GET /auctions/:id`
  - `PATCH /auctions/:id/state` (`DRAFT -> OPEN -> CLOSED` rules)
  - `GET /internal/auctions/:id/status` (internal token)
- **Bid Service** (`cmd/bid-service`)
  - `POST /bids` (auth required, amount must be > current highest, auction must be OPEN)
  - `GET /auctions/:id/bids`
  - `GET /auctions/:id/highest`
  - `GET /ws/auctions/:id` (WebSocket real-time bid stream)

All services expose:
- `/healthz`
- `/readyz`
- `/metrics` (Prometheus format)

## Local development (Docker Compose)

### 1) Configure env

```bash
cp .env.example .env
```

### 2) Start stack

```bash
docker compose up --build
```

This starts:
- `postgres` (5432)
- `redis` (6379)
- `user-service` (8080)
- `auction-service` (8081)
- `bid-service` (8082)

## API quickstart with curl

### Register + login

```bash
curl -s -X POST http://localhost:8080/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"seller@example.com","password":"password123","name":"Seller"}'

TOKEN=$(curl -s -X POST http://localhost:8080/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"seller@example.com","password":"password123"}' | jq -r .token)
```

### Create auction

```bash
curl -X POST http://localhost:8081/auctions \
  -H "Authorization: Token <JWT>" \
  -H 'Content-Type: application/json' \
  -d '{"title":"MacBook Pro","description":"M3 Pro 16-inch","start_time":"2026-01-01T10:00:00Z","end_time":"2026-01-01T12:00:00Z"}'
```

### Open auction

```bash
curl -X PATCH http://localhost:8081/auctions/1/state \
  -H "Authorization: Token <JWT>" \
  -H 'Content-Type: application/json' \
  -d '{"state":"OPEN"}'
```

### Place bid

```bash
curl -X POST http://localhost:8082/bids \
  -H "Authorization: Token <JWT>" \
  -H 'Content-Type: application/json' \
  -d '{"auction_id":1,"amount":1200.00}'
```

### WebSocket live bids

```bash
wscat -c ws://localhost:8082/ws/auctions/1
```

When a bid is placed, subscribers receive:

```json
{"event":"bid_placed","bid":{...}}
```

## Database migrations

Versioned SQL migrations are under `/migrations`:
- `001_users.*.sql`
- `002_auctions.*.sql`
- `003_bids.*.sql`

Use your migration tool of choice (e.g., golang-migrate) or execute the SQL files in order.

## Kubernetes deployment (`k8s/`)

Included manifests:
- `namespace.yaml`
- `configmap.yaml`
- `secret-template.yaml`
- `postgres.yaml`
- `redis.yaml`
- `user-service.yaml`
- `auction-service.yaml`
- `bid-service.yaml`

### Generic Kubernetes / EKS path

1. Build and push images (GHCR/ECR).
2. Update image tags in `k8s/*-service.yaml`.
3. Create namespace + config + secrets:
   ```bash
   kubectl apply -f k8s/namespace.yaml
   kubectl apply -f k8s/configmap.yaml
   kubectl apply -f k8s/secret-template.yaml
   ```
4. Deploy data layer + services:
   ```bash
   kubectl apply -f k8s/postgres.yaml
   kubectl apply -f k8s/redis.yaml
   kubectl apply -f k8s/user-service.yaml
   kubectl apply -f k8s/auction-service.yaml
   kubectl apply -f k8s/bid-service.yaml
   ```

## CI/CD

GitHub Actions workflow: `.github/workflows/ci.yml`
- gofmt check
- go vet
- unit tests
- go build
- Docker image build validation for each service
- Optional GHCR publish on `main`

## Project structure

```text
cmd/
  user-service/
  auction-service/
  bid-service/
internal/
  user/
  auction/
  bid/
  platform/
pkg/
  auth/
  auction/
  bid/
migrations/
k8s/
.github/workflows/
```
