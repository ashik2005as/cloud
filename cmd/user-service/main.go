package main

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/ashik2005as/cloud/internal/platform"
	"github.com/ashik2005as/cloud/internal/user"
	_ "github.com/lib/pq"
)

func main() {
	port := getenv("PORT", "8080")
	dsn := getenv("DATABASE_URL", "host=localhost port=5432 user=postgres dbname=cloud sslmode=disable")
	jwtSecret := getenv("JWT_SECRET", "dev-secret")

	logger := platform.NewLogger("user-service")
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(err)
	}
	repo := user.NewRepository(db)
	if err := repo.Init(); err != nil {
		panic(err)
	}
	r := platform.NewRouter(logger)
	platform.HealthEndpoints(r, db, nil)
	user.NewHandler(repo, jwtSecret).RegisterRoutes(r)
	logger.Info("starting service", "port", port)
	_ = r.Run(fmt.Sprintf(":%s", port))
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
