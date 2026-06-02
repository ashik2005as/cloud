package bid

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ashik2005as/cloud/internal/platform"
	rules "github.com/ashik2005as/cloud/pkg/bid"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

type Bid struct {
	ID        int64     `json:"id"`
	AuctionID int64     `json:"auction_id"`
	BidderID  int64     `json:"bidder_id"`
	Amount    float64   `json:"amount"`
	CreatedAt time.Time `json:"created_at"`
}

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Init() error {
	_, err := r.db.Exec(`CREATE TABLE IF NOT EXISTS bids (
id BIGSERIAL PRIMARY KEY,
auction_id BIGINT NOT NULL,
bidder_id BIGINT NOT NULL,
amount NUMERIC(12,2) NOT NULL,
created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`)
	return err
}

func (r *Repository) Highest(auctionID int64) (float64, error) {
	var v sql.NullFloat64
	if err := r.db.QueryRow(`SELECT MAX(amount) FROM bids WHERE auction_id=$1`, auctionID).Scan(&v); err != nil {
		return 0, err
	}
	if !v.Valid {
		return 0, nil
	}
	return v.Float64, nil
}

func (r *Repository) Create(b *Bid) error {
	return r.db.QueryRow(`INSERT INTO bids(auction_id,bidder_id,amount) VALUES($1,$2,$3) RETURNING id,created_at`, b.AuctionID, b.BidderID, b.Amount).
		Scan(&b.ID, &b.CreatedAt)
}

func (r *Repository) History(auctionID int64) ([]Bid, error) {
	rows, err := r.db.Query(`SELECT id,auction_id,bidder_id,amount,created_at FROM bids WHERE auction_id=$1 ORDER BY created_at DESC`, auctionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Bid
	for rows.Next() {
		var b Bid
		if err := rows.Scan(&b.ID, &b.AuctionID, &b.BidderID, &b.Amount, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

type AuctionStatusClient interface {
	IsAuctionOpen(auctionID int64) (bool, error)
}

type HTTPAuctionStatusClient struct {
	BaseURL       string
	InternalToken string
	HTTPClient    *http.Client
}

func (c *HTTPAuctionStatusClient) IsAuctionOpen(auctionID int64) (bool, error) {
	url := fmt.Sprintf("%s/internal/auctions/%d/status", c.BaseURL, auctionID)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-Internal-Token", c.InternalToken)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, errors.New("auction is not available")
	}
	var payload struct {
		IsOpen bool `json:"is_open"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false, err
	}
	return payload.IsOpen, nil
}

type Hub struct {
	mu      sync.RWMutex
	clients map[int64]map[*websocket.Conn]struct{}
}

func NewHub() *Hub { return &Hub{clients: map[int64]map[*websocket.Conn]struct{}{}} }

func (h *Hub) Subscribe(auctionID int64, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[auctionID] == nil {
		h.clients[auctionID] = map[*websocket.Conn]struct{}{}
	}
	h.clients[auctionID][c] = struct{}{}
}

func (h *Hub) Unsubscribe(auctionID int64, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[auctionID], c)
	_ = c.Close()
}

func (h *Hub) Broadcast(auctionID int64, bid Bid) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients[auctionID] {
		_ = c.WriteJSON(gin.H{"event": "bid_placed", "bid": bid})
	}
}

type Service struct {
	repo          *Repository
	redis         *redis.Client
	auctionClient AuctionStatusClient
	hub           *Hub
}

func NewService(repo *Repository, redis *redis.Client, auctionClient AuctionStatusClient, hub *Hub) *Service {
	return &Service{repo: repo, redis: redis, auctionClient: auctionClient, hub: hub}
}

func (s *Service) Highest(c *gin.Context, auctionID int64) (float64, error) {
	key := fmt.Sprintf("highest:%d", auctionID)
	if s.redis != nil {
		if raw, err := s.redis.Get(c, key).Result(); err == nil {
			v, parseErr := strconv.ParseFloat(raw, 64)
			if parseErr == nil {
				return v, nil
			}
		}
	}
	v, err := s.repo.Highest(auctionID)
	if err != nil {
		return 0, err
	}
	if s.redis != nil {
		s.redis.Set(c, key, fmt.Sprintf("%.2f", v), 15*time.Second)
	}
	return v, nil
}

func (s *Service) Place(c *gin.Context, b *Bid) error {
	open, err := s.auctionClient.IsAuctionOpen(b.AuctionID)
	if err != nil || !open {
		return errors.New("auction is not open")
	}
	highest, err := s.Highest(c, b.AuctionID)
	if err != nil {
		return err
	}
	if !rules.ValidBid(b.Amount, highest) {
		return errors.New("bid amount must be greater than current highest")
	}
	if err := s.repo.Create(b); err != nil {
		return err
	}
	if s.redis != nil {
		s.redis.Del(c, fmt.Sprintf("highest:%d", b.AuctionID))
	}
	s.hub.Broadcast(b.AuctionID, *b)
	return nil
}

type Handler struct {
	svc       *Service
	repo      *Repository
	hub       *Hub
	jwtSecret string
}

func NewHandler(svc *Service, repo *Repository, hub *Hub, jwtSecret string) *Handler {
	return &Handler{svc: svc, repo: repo, hub: hub, jwtSecret: jwtSecret}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	auth := r.Group("", platform.RequireAuth(h.jwtSecret))
	auth.POST("/bids", h.place)
	r.GET("/auctions/:id/bids", h.history)
	r.GET("/auctions/:id/highest", h.highest)
	r.GET("/ws/auctions/:id", h.subscribe)
}

func auctionID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid auction id"})
		return 0, false
	}
	return id, true
}

func (h *Handler) place(c *gin.Context) {
	uid, ok := platform.UserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req struct {
		AuctionID int64   `json:"auction_id" binding:"required"`
		Amount    float64 `json:"amount" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	b := &Bid{AuctionID: req.AuctionID, BidderID: uid, Amount: req.Amount}
	if err := h.svc.Place(c, b); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, b)
}

func (h *Handler) history(c *gin.Context) {
	id, ok := auctionID(c)
	if !ok {
		return
	}
	items, err := h.repo.History(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch bid history"})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) highest(c *gin.Context) {
	id, ok := auctionID(c)
	if !ok {
		return
	}
	v, err := h.svc.Highest(c, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch highest bid"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"auction_id": id, "highest_bid": v})
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func (h *Handler) subscribe(c *gin.Context) {
	id, ok := auctionID(c)
	if !ok {
		return
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.hub.Subscribe(id, conn)
	defer h.hub.Unsubscribe(id, conn)
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
