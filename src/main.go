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
		log.Println("Warning: No .env file found, using system env")
	}

	e := echo.New()

	// Database connection
	config.ConnectDB()

	// Konfigurasi Middleware
	e.Use(middleware.Logger()) // Tambahkan logger agar debug lebih mudah di Ubuntu Server
	e.Use(middleware.Recover())

	// CORS Configuration
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		// Tambahkan IP Ubuntu Server (10.0.1.56) ke daftar origin
		AllowOrigins: []string{
			"http://localhost:5173",
			"http://127.0.0.1:5173",
			"http://10.0.1.56:5173", // URL yang akan diakses laptop kamu/rekan kantor
		},
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodDelete,
			http.MethodPatch,
		},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAuthorization,
		},
	}))

	// Static file serving untuk uploads
	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	e.Static("/uploads", uploadDir)

	// Register Routes
	routes.UserRoutes(e)
	routes.ActivityRoutes(e)
	routes.KPIRoutes(e)

	// Menjalankan server pada port 8080
	// Menggunakan ":8080" secara otomatis bind ke 0.0.0.0 (semua interface)
	e.Logger.Fatal(e.Start(":8080"))
}
