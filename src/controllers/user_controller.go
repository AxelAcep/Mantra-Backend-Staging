package controllers

import (
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"mantra/src/config"
	"mantra/src/middleware"
	"mantra/src/models"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ==========================================
// HELPER
// ==========================================

func generateID(jenis string) string {
	const characters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	result := make([]byte, 6)
	for i := range result {
		result[i] = characters[r.Intn(len(characters))]
	}
	switch jenis {
	case "user":
		return "USR" + string(result)
	case "pegawai":
		return "PG" + string(result)
	default:
		return string(result)
	}
}

func isDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return containsStr(s, "duplicate key") || containsStr(s, "23505")
}

func containsStr(s, sub string) bool {
	if len(s) < len(sub) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ==========================================
// REGISTER
// ==========================================

type RegisterRequest struct {
	Nama     string `json:"nama"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
	Divisi   string `json:"divisi"`
}

func Register(c echo.Context) error {
	var req RegisterRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Format request tidak valid."})
	}
	if req.Nama == "" || req.Email == "" || req.Password == "" || req.Role == "" || req.Divisi == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Input tidak lengkap."})
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	var pegawai models.Pegawai
	var user models.User

	err = config.DB.Transaction(func(tx *gorm.DB) error {
		pegawai = models.Pegawai{
			ID:     generateID("pegawai"),
			Nama:   req.Nama,
			Divisi: models.Divisi(req.Divisi),
		}
		if err := tx.Create(&pegawai).Error; err != nil {
			return err
		}

		user = models.User{
			ID:        generateID("user"),
			Email:     req.Email,
			Password:  string(hashedPassword),
			Role:      models.Role(req.Role),
			PegawaiID: pegawai.ID,
		}
		return tx.Create(&user).Error
	})

	if err != nil {
		if isDuplicateError(err) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "Email sudah terdaftar."})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"message": "Akun berhasil dibuat.",
		"user": map[string]interface{}{
			"id":        user.ID,
			"email":     user.Email,
			"role":      user.Role,
			"pegawaiId": user.PegawaiID,
		},
		"pegawai": map[string]interface{}{
			"id":     pegawai.ID,
			"nama":   pegawai.Nama,
			"divisi": pegawai.Divisi,
		},
	})
}

// ==========================================
// LOGIN
// ==========================================

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Format request tidak valid."})
	}
	if req.Email == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Email dan password wajib diisi."})
	}

	var user models.User
	if err := config.DB.Preload("Pegawai").Where("email = ?", req.Email).First(&user).Error; err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Email atau password salah."})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Email atau password salah."})
	}

	claims := jwt.MapClaims{
		"id":    user.ID,
		"email": user.Email,
		"role":  user.Role,
		"pegawai": map[string]interface{}{
			"id":     user.Pegawai.ID,
			"nama":   user.Pegawai.Nama,
			"divisi": user.Pegawai.Divisi,
		},
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	now := time.Now()
	config.DB.Model(&user).Update("last_login", now)
	middleware.ResetLoginAttempts(c.RealIP())

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Login berhasil.",
		"token":   signedToken,
		"user": map[string]interface{}{
			"id":       user.ID,
			"email":    user.Email,
			"role":     user.Role,
			"isActive": true,
			"pegawai": map[string]interface{}{
				"id":     user.Pegawai.ID,
				"nama":   user.Pegawai.Nama,
				"divisi": user.Pegawai.Divisi,
			},
		},
	})
}

// ==========================================
// GET ALL USERS
// ==========================================

func GetAllUsers(c echo.Context) error {
	page := 1
	limit := 10

	if p := c.QueryParam("page"); p != "" {
		if val, err := strconv.Atoi(p); err == nil && val > 0 {
			page = val
		}
	}
	if l := c.QueryParam("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	search := c.QueryParam("search")
	offset := (page - 1) * limit

	query := config.DB.Model(&models.User{}).Joins("JOIN \"Pegawai\" ON \"Pegawai\".id = \"User\".pegawai_id")

	if search != "" {
		like := "%" + search + "%"
		query = query.Where("\"Pegawai\".nama ILIKE ? OR \"User\".email ILIKE ?", like, like)
	}

	var total int64
	query.Count(&total)

	var users []models.User
	if err := query.Preload("Pegawai").Limit(limit).Offset(offset).Find(&users).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Data user berhasil diambil.",
		"data":    users,
		"meta": map[string]interface{}{
			"page":       page,
			"limit":      limit,
			"total":      total,
			"totalPages": totalPages,
		},
	})
}

// ==========================================
// GET ONE USER
// ==========================================

func GetOneUser(c echo.Context) error {
	id := c.Param("id")

	var user models.User
	if err := config.DB.Preload("Pegawai").Where("id = ?", id).First(&user).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User tidak ditemukan."})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Data user berhasil diambil.",
		"data":    user,
	})
}

// ==========================================
// EDIT USER
// ==========================================

type EditUserRequest struct {
	Nama     string `json:"nama"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Divisi   string `json:"divisi"`
	Password string `json:"password"`
}

func EditUser(c echo.Context) error {
	id := c.Param("id")

	var req EditUserRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Format request tidak valid."})
	}

	var user models.User
	if err := config.DB.Preload("Pegawai").Where("id = ?", id).First(&user).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User tidak ditemukan."})
	}

	if req.Password != "" {
		hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
		if err != nil {
			return err
		}
		user.Password = string(hashed)
	}

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		if req.Nama != "" {
			user.Pegawai.Nama = req.Nama
		}
		if req.Divisi != "" {
			user.Pegawai.Divisi = models.Divisi(req.Divisi)
		}
		if err := tx.Save(&user.Pegawai).Error; err != nil {
			return err
		}

		if req.Email != "" {
			user.Email = req.Email
		}
		if req.Role != "" {
			user.Role = models.Role(req.Role)
		}
		return tx.Save(&user).Error
	})

	if err != nil {
		if isDuplicateError(err) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "Email sudah digunakan."})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Data user berhasil diperbarui.",
		"data":    user,
	})
}

// ==========================================
// DELETE USER
// ==========================================

func DeleteUser(c echo.Context) error {
	id := c.Param("id")

	var user models.User
	if err := config.DB.Preload("Pegawai").Where("id = ?", id).First(&user).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "User tidak ditemukan."})
	}

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&user).Error; err != nil {
			return err
		}
		return tx.Delete(&user.Pegawai).Error
	})

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Terjadi kesalahan pada server."})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "User berhasil dihapus.",
	})
}

func GetAllPegawai(c echo.Context) error {
	var pegawais []models.Pegawai

	if err := config.DB.
		Model(&models.Pegawai{}).
		Select("id, nama, divisi").
		Where("deleted_at IS NULL").
		Order("nama ASC").
		Find(&pegawais).Error; err != nil {

		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Gagal mengambil data pegawai.",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data":    pegawais,
		"message": "Berhasil mengambil data pegawai.",
	})
}
