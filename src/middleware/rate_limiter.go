package middleware

import (
    "net/http"
    "sync"
    "time"

    "github.com/labstack/echo/v4"
)

// ==========================================
// STRUKTUR TRACKER PER IP
// ==========================================

type rateLimitEntry struct {
    count     int
    resetTime time.Time
}

var (
    loginAttempts = make(map[string]*rateLimitEntry)
    mu            sync.Mutex
)

const (
    maxAttempts = 5
    windowDur   = 60 * 60 * time.Second // 1 jam
)

// ==========================================
// LOGIN RATE LIMITER
// skipSuccessfulRequests = true
// artinya middleware ini hanya dipanggil saat LOGIN GAGAL
// ==========================================

func LoginRateLimiter(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c echo.Context) error {
        ip := c.RealIP()

        mu.Lock()
        entry, exists := loginAttempts[ip]

        // Reset kalau window sudah lewat
        if exists && time.Now().After(entry.resetTime) {
            delete(loginAttempts, ip)
            exists = false
        }

        if !exists {
            loginAttempts[ip] = &rateLimitEntry{
                count:     0,
                resetTime: time.Now().Add(windowDur),
            }
            entry = loginAttempts[ip]
        }

        // Cek apakah sudah melebihi batas
        if entry.count >= maxAttempts {
            retryAfter := int(time.Until(entry.resetTime).Seconds())
            mu.Unlock()
            return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
                "error":              "Terlalu banyak percobaan login. Coba lagi dalam 1 jam.",
                "retryAfterSeconds":  retryAfter,
            })
        }
        mu.Unlock()

        // Jalankan handler login
        err := next(c)

        // Hitung attempt HANYA kalau gagal (status bukan 200)
        if c.Response().Status != http.StatusOK {
            mu.Lock()
            loginAttempts[ip].count++
            mu.Unlock()
        }

        return err
    }
}

// ==========================================
// RESET — dipanggil saat login berhasil (opsional)
// ==========================================

func ResetLoginAttempts(ip string) {
    mu.Lock()
    defer mu.Unlock()
    delete(loginAttempts, ip)
}