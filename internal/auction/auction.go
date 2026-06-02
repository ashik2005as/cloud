package auction

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ashik2005as/cloud/internal/platform"
	rules "github.com/ashik2005as/cloud/pkg/auction"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type Auction struct {
	ID          int64     `json:"id"`
	SellerID    int64     `json:"seller_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Init() error {
	_, err := r.db.Exec(`CREATE TABLE IF NOT EXISTS auctions (
id BIGSERIAL PRIMARY KEY,
seller_id BIGINT NOT NULL,
title TEXT NOT NULL,
description TEXT NOT NULL,
start_time TIMESTAMPTZ NOT NULL,
end_time TIMESTAMPTZ NOT NULL,
state TEXT NOT NULL,
created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`)
	return err
}

func (r *Repository) Create(a *Auction) error {
	return r.db.QueryRow(`INSERT INTO auctions(seller_id,title,description,start_time,end_time,state)
VALUES($1,$2,$3,$4,$5,$6) RETURNING id,created_at,updated_at`, a.SellerID, a.Title, a.Description, a.StartTime, a.EndTime, a.State).
		Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

func (r *Repository) List() ([]Auction, error) {
	rows, err := r.db.Query(`SELECT id,seller_id,title,description,start_time,end_time,state,created_at,updated_at FROM auctions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Auction
	for rows.Next() {
		var a Auction
		if err := rows.Scan(&a.ID, &a.SellerID, &a.Title, &a.Description, &a.StartTime, &a.EndTime, &a.State, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (r *Repository) ByID(id int64) (*Auction, error) {
	row := r.db.QueryRow(`SELECT id,seller_id,title,description,start_time,end_time,state,created_at,updated_at FROM auctions WHERE id=$1`, id)
	var a Auction
	if err := row.Scan(&a.ID, &a.SellerID, &a.Title, &a.Description, &a.StartTime, &a.EndTime, &a.State, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repository) UpdateState(id int64, state string) error {
	_, err := r.db.Exec(`UPDATE auctions SET state=$1, updated_at=NOW() WHERE id=$2`, state, id)
	return err
}

type Service struct {
	repo  *Repository
	redis *redis.Client
}

func NewService(repo *Repository, redis *redis.Client) *Service {
	return &Service{repo: repo, redis: redis}
}

func (s *Service) List(ctx context.Context) ([]Auction, error) {
	if s.redis != nil {
		if raw, err := s.redis.Get(ctx, "auctions:list").Result(); err == nil {
			var cached []Auction
			if json.Unmarshal([]byte(raw), &cached) == nil {
				return cached, nil
			}
		}
	}
	items, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	if s.redis != nil {
		if raw, err := json.Marshal(items); err == nil {
			s.redis.Set(ctx, "auctions:list", raw, 30*time.Second)
		}
	}
	return items, nil
}

func (s *Service) ByID(ctx context.Context, id int64) (*Auction, error) {
	key := fmt.Sprintf("auction:%d", id)
	if s.redis != nil {
		if raw, err := s.redis.Get(ctx, key).Result(); err == nil {
			var a Auction
			if json.Unmarshal([]byte(raw), &a) == nil {
				return &a, nil
			}
		}
	}
	a, err := s.repo.ByID(id)
	if err != nil {
		return nil, err
	}
	if s.redis != nil {
		if raw, err := json.Marshal(a); err == nil {
			s.redis.Set(ctx, key, raw, 30*time.Second)
		}
	}
	return a, nil
}

func (s *Service) invalidate(ctx context.Context, id int64) {
	if s.redis == nil {
		return
	}
	s.redis.Del(ctx, "auctions:list", fmt.Sprintf("auction:%d", id))
}

type Handler struct {
	svc           *Service
	repo          *Repository
	jwtSecret     string
	internalToken string
}

func NewHandler(svc *Service, repo *Repository, jwtSecret, internalToken string) *Handler {
	return &Handler{svc: svc, repo: repo, jwtSecret: jwtSecret, internalToken: internalToken}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	r.GET("/auctions", h.list)
	r.GET("/auctions/:id", h.detail)
	auth := r.Group("", platform.RequireAuth(h.jwtSecret))
	auth.POST("/auctions", h.create)
	auth.PATCH("/auctions/:id/state", h.changeState)
	internal := r.Group("/internal", platform.RequireInternalToken(h.internalToken))
	internal.GET("/auctions/:id/status", h.internalStatus)
}

func (h *Handler) create(c *gin.Context) {
	uid, ok := platform.UserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req struct {
		Title       string    `json:"title" binding:"required"`
		Description string    `json:"description" binding:"required"`
		StartTime   time.Time `json:"start_time" binding:"required"`
		EndTime     time.Time `json:"end_time" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !req.EndTime.After(req.StartTime) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "end_time must be after start_time"})
		return
	}
	a := &Auction{SellerID: uid, Title: req.Title, Description: req.Description, StartTime: req.StartTime, EndTime: req.EndTime, State: rules.StateDraft}
	if err := h.repo.Create(a); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create auction"})
		return
	}
	h.svc.invalidate(c, a.ID)
	c.JSON(http.StatusCreated, a)
}

func (h *Handler) list(c *gin.Context) {
	items, err := h.svc.List(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list auctions"})
		return
	}
	c.JSON(http.StatusOK, items)
}

func parseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return 0, false
	}
	return id, true
}

func (h *Handler) detail(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	a, err := h.svc.ByID(c, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "auction not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch auction"})
		return
	}
	c.JSON(http.StatusOK, a)
}

func (h *Handler) changeState(c *gin.Context) {
	uid, ok := platform.UserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id, ok := parseID(c)
	if !ok {
		return
	}
	a, err := h.repo.ByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "auction not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch auction"})
		return
	}
	if a.SellerID != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "only seller can manage auction"})
		return
	}
	var req struct {
		State string `json:"state" binding:"required,oneof=OPEN CLOSED"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !rules.CanTransition(a.State, req.State, time.Now().UTC(), a.StartTime, a.EndTime) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state transition"})
		return
	}
	if err := h.repo.UpdateState(a.ID, req.State); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update state"})
		return
	}
	h.svc.invalidate(c, a.ID)
	a.State = req.State
	c.JSON(http.StatusOK, a)
}

func (h *Handler) internalStatus(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	a, err := h.repo.ByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "auction not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch auction"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":         a.ID,
		"state":      a.State,
		"start_time": a.StartTime,
		"end_time":   a.EndTime,
		"is_open":    rules.IsOpen(a.State, time.Now().UTC(), a.StartTime, a.EndTime),
	})
}
