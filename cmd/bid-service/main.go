package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ashik2005as/cloud/internal/bid"
	"github.com/ashik2005as/cloud/internal/platform"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func main() {
	port := getenv("PORT", "8082")
	dsn := getenv("DATABASE_URL", "host=localhost port=5432 user=postgres dbname=cloud sslmode=disable")
	jwtSecret := getenv("JWT_SECRET", "dev-secret")
	internalToken := getenv("INTERNAL_SERVICE_TOKEN", "internal-dev-token")
	auctionURL := getenv("AUCTION_SERVICE_URL", "http://localhost:8081")
	redisAddr := getenv("REDIS_ADDR", "localhost:6379")

	logger := platform.NewLogger("bid-service")
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(err)
	}
	var redisClient *redis.Client
	if redisAddr != "" {
		redisClient = redis.NewClient(&redis.Options{Addr: redisAddr})
	}
	repo := bid.NewRepository(db)
	if err := repo.Init(); err != nil {
		panic(err)
	}
	hub := bid.NewHub()
	auctionClient := &bid.HTTPAuctionStatusClient{
		BaseURL:       auctionURL,
		InternalToken: internalToken,
		HTTPClient:    &http.Client{Timeout: 3 * time.Second},
	}
	svc := bid.NewService(repo, redisClient, auctionClient, hub)

	r := platform.NewRouter(logger)
	platform.HealthEndpoints(r, db, redisClient)
	bid.NewHandler(svc, repo, hub, jwtSecret).RegisterRoutes(r)
	logger.Info("starting service", "port", port)
	_ = r.Run(fmt.Sprintf(":%s", port))
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
