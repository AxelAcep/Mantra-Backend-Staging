package middleware

import (
    "fmt"
    "net/http"
    "os"
    "strings"

    "github.com/golang-jwt/jwt/v5"
    "github.com/labstack/echo/v4"
)

// ==========================================
// ROLE LEVEL
// ==========================================

var roleLevel = map[string]int{
    "MASTER":    1,
    "SUPERVISI": 2,
    "PROJEK":    3,
    "KARYAWAN":  4,
}

// ==========================================
// VERIFY TOKEN
// ==========================================

func VerifyToken(next echo.HandlerFunc) echo.HandlerFunc {
    return func(c echo.Context) error {
        authHeader := c.Request().Header.Get("Authorization")
        if authHeader == "" {
            return c.JSON(http.StatusUnauthorized, map[string]string{
                "error": "Token tidak ditemukan. Silakan login.",
            })
        }

        // Format: "Bearer <token>"
        parts := strings.SplitN(authHeader, " ", 2)
        if len(parts) != 2 || parts[0] != "Bearer" {
            return c.JSON(http.StatusUnauthorized, map[string]string{
                "error": "Format token tidak valid.",
            })
        }

        tokenStr := parts[1]

        // Parse & verify token
        token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
            if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("signing method tidak valid")
            }
            return []byte(os.Getenv("JWT_SECRET")), nil
        })

        if err != nil {
            if strings.Contains(err.Error(), "expired") {
                return c.JSON(http.StatusUnauthorized, map[string]string{
                    "error": "Token sudah expired. Silakan login ulang.",
                })
            }
            return c.JSON(http.StatusForbidden, map[string]string{
                "error": "Token tidak valid.",
            })
        }

        // Inject claims ke context
        claims, ok := token.Claims.(jwt.MapClaims)
        if !ok || !token.Valid {
            return c.JSON(http.StatusForbidden, map[string]string{
                "error": "Token tidak valid.",
            })
        }

        c.Set("user", claims)
        return next(c)
    }
}

// ==========================================
// AUTHORIZE ROLE
// ==========================================

func AuthorizeRole(requiredLevel int) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            claims, ok := c.Get("user").(jwt.MapClaims)
            if !ok {
                return c.JSON(http.StatusUnauthorized, map[string]string{
                    "error": "Unauthorized.",
                })
            }

            userRole, ok := claims["role"].(string)
            if !ok || userRole == "" {
                return c.JSON(http.StatusUnauthorized, map[string]string{
                    "error": "Unauthorized.",
                })
            }

            userLevel, exists := roleLevel[userRole]
            if !exists {
                return c.JSON(http.StatusForbidden, map[string]string{
                    "error": "Role tidak dikenali.",
                })
            }

            if userLevel > requiredLevel {
                // Cari nama role minimum yang dibutuhkan
                minRole := ""
                for role, level := range roleLevel {
                    if level == requiredLevel {
                        minRole = role
                        break
                    }
                }
                return c.JSON(http.StatusForbidden, map[string]string{
                    "error": fmt.Sprintf("Akses ditolak. Diperlukan role minimal: %s.", minRole),
                })
            }

            return next(c)
        }
    }
}