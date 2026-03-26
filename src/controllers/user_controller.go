package controllers

import (
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/smtp"
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

type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

func ForgotPassword(c echo.Context) error {
	type Request struct {
		Email string `json:"email"`
	}
	var req Request
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Format request tidak valid."})
	}

	// Cari user berdasarkan email
	var user models.User
	if err := config.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Email tidak terdaftar."})
	}

	// FILTER KHUSUS MASTER
	if user.Role != "MASTER" {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Akses ditolak. Fitur ini hanya untuk akun Master."})
	}

	// Generate Token & Expiry (1 jam)
	token := generateRandomToken(30)
	expiry := time.Now().Add(1 * time.Hour)

	// Simpan token ke database
	user.ResetToken = token
	user.TokenExpires = &expiry
	if err := config.DB.Save(&user).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal memproses token reset."})
	}

	// --- KONFIGURASI SMTP GMAIL ---
	from := "anfsel13@gmail.com" // Ganti dengan email pengirim
	password := "rieb uhjb qhdo fosw"
	smtpHost := "smtp.gmail.com"
	smtpPort := "587"

	// Link yang akan dikirim ke email
	resetLink := "http://localhost:5173/reset-password?token=" + token

	// Draft Email (HTML)
	subject := "Subject: [MASTER] Atur Ulang Kata Sandi CRM\n"
	mime := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"
	body := fmt.Sprintf(`
		<div style="font-family: sans-serif; max-width: 500px; border: 1px solid #eee; padding: 20px;">
			<h2 style="color: #06B6D4;">Halo, Master!</h2>
			<p>Anda menerima email ini karena ada permintaan pengaturan ulang kata sandi untuk akun Master Anda.</p>
			<p>Silakan klik tombol di bawah ini (berlaku 1 jam):</p>
			<a href="%s" style="display: inline-block; padding: 10px 20px; background-color: #06B6D4; color: white; text-decoration: none; border-radius: 5px; font-weight: bold;">
				Atur Ulang Password
			</a>
			<p style="margin-top: 20px; color: #888; font-size: 12px;">Jika Anda tidak merasa meminta ini, abaikan email ini.</p>
			<hr>
			<p style="font-size: 10px; color: #aaa;">© 2026 CRM PT. Matur Nuwun Nusantara</p>
		</div>`, resetLink)

	msg := []byte(subject + mime + body)
	auth := smtp.PlainAuth("", from, password, smtpHost)

	// Kirim Email
	err := smtp.SendMail(smtpHost+":"+smtpPort, auth, from, []string{user.Email}, msg)
	if err != nil {
		fmt.Println("SMTP Error:", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengirim email reset."})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Link reset password telah dikirim ke email Anda.",
	})
}

func ResetPassword(c echo.Context) error {
	type ResetReq struct {
		Token          string `json:"token"`
		PasswordBaru   string `json:"password_baru"`
		KonfirmasiPass string `json:"konfirmasi_password"`
	}

	var req ResetReq
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Data tidak valid."})
	}

	if req.PasswordBaru != req.KonfirmasiPass {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Password tidak cocok."})
	}

	// 1. Cari user berdasarkan token & pastikan token belum expired
	var user models.User
	err := config.DB.Where("reset_token = ? AND token_expires > ?", req.Token, time.Now()).First(&user).Error

	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Token tidak valid atau sudah kedaluwarsa."})
	}

	// 2. Hash Password Baru
	hashed, _ := bcrypt.GenerateFromPassword([]byte(req.PasswordBaru), 10)

	// 3. Update Password dan Hapus Token agar tidak bisa dipakai lagi
	user.Password = string(hashed)
	user.ResetToken = ""    // Kosongkan token
	user.TokenExpires = nil // Kosongkan expiry

	if err := config.DB.Save(&user).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal update password."})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Password berhasil diperbarui. Silakan login."})
}

// Helper untuk generate token string
func generateRandomToken(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
