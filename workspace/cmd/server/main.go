package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"

	"mini-server/internal/handler"
	"mini-server/internal/middleware"
)

func main() {
	r := chi.NewRouter()

	// 全局中间件
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Use(middleware.Logging)

	// 注册路由
	r.Get("/", handler.Home)
	r.Get("/health", handler.Health)
	r.Get("/api/info", handler.APIInfo)

	addr := ":8080"
	fmt.Printf("🚀 Server starting on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}
