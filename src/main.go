package main

import (
	"log"
	"mantra/src/config"
	"mantra/src/routes"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file, using system env")
	}

	e := echo.New()
	config.ConnectDB()

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodDelete,
			http.MethodPatch,
			http.MethodOptions,
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
			"X-Requested-With",
		},
		AllowCredentials: false,
	}))

	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	e.Static("/uploads", uploadDir)

	routes.UserRoutes(e)
	routes.ActivityRoutes(e)
	routes.KPIRoutes(e)
	routes.PerusahaanRoutes(e)

	e.Logger.Fatal(e.Start(":8080"))
}
