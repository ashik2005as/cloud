package main

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/ashik2005as/cloud/internal/auction"
	"github.com/ashik2005as/cloud/internal/platform"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func main() {
	port := getenv("PORT", "8081")
	dsn := getenv("DATABASE_URL", "host=localhost port=5432 user=postgres dbname=cloud sslmode=disable")
	jwtSecret := getenv("JWT_SECRET", "dev-secret")
	internalToken := getenv("INTERNAL_SERVICE_TOKEN", "internal-dev-token")
	redisAddr := getenv("REDIS_ADDR", "localhost:6379")

	logger := platform.NewLogger("auction-service")
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(err)
	}
	var redisClient *redis.Client
	if redisAddr != "" {
		redisClient = redis.NewClient(&redis.Options{Addr: redisAddr})
	}

	repo := auction.NewRepository(db)
	if err := repo.Init(); err != nil {
		panic(err)
	}
	svc := auction.NewService(repo, redisClient)
	r := platform.NewRouter(logger)
	platform.HealthEndpoints(r, db, redisClient)
	auction.NewHandler(svc, repo, jwtSecret, internalToken).RegisterRoutes(r)
	logger.Info("starting service", "port", port)
	_ = r.Run(fmt.Sprintf(":%s", port))
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
