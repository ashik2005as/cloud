package platform

import (
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func NewLogger(service string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})).With("service", service)
}

func NewRouter(logger *slog.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
		)
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	return r
}

func HealthEndpoints(r *gin.Engine, db *sql.DB, redisClient *redis.Client) {
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/readyz", func(c *gin.Context) {
		if db != nil {
			if err := db.Ping(); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db not ready", "error": err.Error()})
				return
			}
		}
		if redisClient != nil {
			if err := redisClient.Ping(c).Err(); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "redis not ready", "error": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
}

func RequireAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		claims, err := ParseJWT(secret, strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Next()
	}
}

func UserID(c *gin.Context) (int64, bool) {
	v, ok := c.Get("user_id")
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case int64:
		return t, true
	case float64:
		return int64(t), true
	case string:
		i, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func RequireInternalToken(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if expected == "" || c.GetHeader("X-Internal-Token") != expected {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}
