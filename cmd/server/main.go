package main

import (
	"context"
	"log"
	"time"

	"github.com/ArminDashti/mark-api/internal/auth"
	"github.com/ArminDashti/mark-api/internal/config"
	appdb "github.com/ArminDashti/mark-api/internal/db"
	"github.com/ArminDashti/mark-api/internal/handlers"
	"github.com/ArminDashti/mark-api/internal/marks"
	"github.com/ArminDashti/mark-api/internal/storage"
	"github.com/ArminDashti/mark-api/internal/users"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	config.LoadDotEnv(".env")

	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	sqlDB, err := appdb.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer sqlDB.Close()

	if err := appdb.Migrate(sqlDB, cfg.MigrationsDir); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	defaultHash, err := auth.HashPassword(appdb.DefaultPassword)
	if err != nil {
		log.Fatalf("hash default password: %v", err)
	}
	if err := appdb.SeedDefaultUser(ctx, sqlDB, defaultHash); err != nil {
		log.Fatalf("seed default user: %v", err)
	}

	if err := storage.EnsureRoot(cfg.DataDir); err != nil {
		log.Fatalf("data dir: %v", err)
	}
	disk, err := storage.NewDisk(cfg.DataDir)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}

	markStore := marks.NewStore(sqlDB)
	markSvc := marks.NewService(markStore, disk, cfg.DataDir)
	userStore := users.NewStore(sqlDB)
	h := handlers.New(cfg, userStore, markSvc)

	r := gin.Default()
	r.MaxMultipartMemory = 8 << 20
	r.Use(cors.New(cors.Config{
		AllowOriginFunc: func(origin string) bool {
			return true
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	r.GET("/health", h.Health)
	r.GET("/m/:kind/:slug", h.ServeMark)

	api := r.Group("/api/v1")
	{
		api.POST("/auth/login", h.Login)

		authed := api.Group("")
		authed.Use(auth.Middleware(cfg.JWTSecret))
		{
			authed.GET("/marks", h.ListMarks)
			authed.POST("/marks", h.CreateMark)
			authed.PUT("/marks/:id", h.UpdateMark)
			authed.DELETE("/marks/:id", h.DeleteMark)
		}
	}

	log.Printf("mark-api listening on %s", cfg.Addr)
	if err := r.Run(cfg.Addr); err != nil {
		log.Fatalf("server: %v", err)
	}
}
