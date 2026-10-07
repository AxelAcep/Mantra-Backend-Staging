package controllers

import (
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"mantra/src/config"
	"mantra/src/models"
)

// Dashboard Accounting — menu sidebar terpisah (bukan tab per-PO yang udah
// ada), buat monitoring seluruh termin pembayaran lintas PO sekaligus:
// ringkasan, highlight termin mendekati/lewat tenggat, dan tabel semua PO.

// ─── Helpers ──────────────────────────────────────────────────────────────────

func getAccountingDashboardClaims(c echo.Context) (roleStr, divisiStr string, ok bool) {
	claims, valid := c.Get("user").(jwt.MapClaims)
	if !valid {
		return "", "", false
	}
	pegawaiMap, _ := claims["pegawai"].(map[string]interface{})
	roleStr, _ = claims["role"].(string)
	divisiStr, _ = pegawaiMap["divisi"].(string)
	return roleStr, divisiStr, true
}

func isAccountingDashboardAuthorized(roleStr, divisiStr string) bool {
	if roleStr == "MASTER" {
		return true
	}
	switch divisiStr {
	case "FINANCE_ACCOUNTING", "MANAGER_OPERASIONAL", "DIREKTUR", "KOMISARIS":
		return true
	}
	return false
}

func flagPriority(flag string) int {
	switch flag {
	case "LEWAT":
		return 0
	case "1_MINGGU":
		return 1
	case "2_MINGGU":
		return 2
	default:
		return 3
	}
}

// hitungFlag menentukan flag tenggat item termin yang belum dibayar — logika
// sama persis dengan GetAccounting (accounting_controller.go), biar konsisten
// antara tab per-PO & dashboard ini.
func hitungFlag(deadline *time.Time, sudahDibayar bool, now, oneWeek, twoWeeks time.Time) string {
	if deadline == nil || sudahDibayar {
		return ""
	}
	switch {
	case deadline.Before(now):
		return "LEWAT"
	case deadline.Before(oneWeek):
		return "1_MINGGU"
	case deadline.Before(twoWeeks):
		return "2_MINGGU"
	default:
		return ""
	}
}

// ─── GET /accounting/summary ──────────────────────────────────────────────────

type AccountingHighlightItem struct {
	ItemTerminID   string    `json:"itemTerminId"`
	TrackingID     string    `json:"trackingId"`
	NomorPenawaran string    `json:"nomorPenawaran"`
	PerusahaanName string    `json:"perusahaanName"`
	NamaTermin     string    `json:"namaTermin"`
	Persentase     float64   `json:"persentase"`
	Nominal        *float64  `json:"nominal,omitempty"`
	Deadline       time.Time `json:"deadline"`
	Flag           string    `json:"flag"` // LEWAT | 1_MINGGU | 2_MINGGU
	HariTersisa    int       `json:"hariTersisa"` // negatif kalau LEWAT
}

type AccountingSummaryResponse struct {
	TotalPO          int64                      `json:"totalPO"`
	TotalTerminBelum int64                      `json:"totalTerminBelum"`
	TotalTerminSudah int64                      `json:"totalTerminSudah"`
	TotalLewat       int64                      `json:"totalLewat"`
	TotalMendekati   int64                      `json:"totalMendekati"` // 1_MINGGU + 2_MINGGU
	Highlights       []AccountingHighlightItem  `json:"highlights"`
}

func GetAccountingSummary(c echo.Context) error {
	roleStr, divisiStr, ok := getAccountingDashboardClaims(c)
	if !ok || !isAccountingDashboardAuthorized(roleStr, divisiStr) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Tidak punya akses ke dashboard Accounting."})
	}

	var termins []models.TerminPembayaran
	if err := config.DB.
		Preload("Items").
		Preload("TrackingPenawaran.Perusahaan").
		Preload("TrackingPenawaran.PenyusunanBoQ").
		Find(&termins).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil data accounting."})
	}

	now := time.Now()
	oneWeek := now.Add(7 * 24 * time.Hour)
	twoWeeks := now.Add(14 * 24 * time.Hour)

	var totalTerminBelum, totalTerminSudah, totalLewat, totalMendekati int64
	highlights := make([]AccountingHighlightItem, 0)

	for _, termin := range termins {
		var estimasi *float64
		if termin.TrackingPenawaran.PenyusunanBoQ != nil {
			estimasi = termin.TrackingPenawaran.PenyusunanBoQ.EstimasiHarga
		}

		for _, item := range termin.Items {
			if item.SudahDibayar {
				totalTerminSudah++
				continue
			}
			totalTerminBelum++

			flag := hitungFlag(item.Deadline, item.SudahDibayar, now, oneWeek, twoWeeks)
			if flag == "" {
				continue
			}
			if flag == "LEWAT" {
				totalLewat++
			} else {
				totalMendekati++
			}

			var nominal *float64
			if estimasi != nil {
				n := *estimasi * item.Persentase / 100
				nominal = &n
			}

			highlights = append(highlights, AccountingHighlightItem{
				ItemTerminID:   item.ID,
				TrackingID:     termin.TrackingPenawaranID,
				NomorPenawaran: termin.TrackingPenawaran.NomorPenawaran,
				PerusahaanName: termin.TrackingPenawaran.Perusahaan.Nama,
				NamaTermin:     item.NamaTermin,
				Persentase:     item.Persentase,
				Nominal:        nominal,
				Deadline:       *item.Deadline,
				Flag:           flag,
				HariTersisa:    int(math.Round(item.Deadline.Sub(now).Hours() / 24)),
			})
		}
	}

	sort.Slice(highlights, func(i, j int) bool {
		pi, pj := flagPriority(highlights[i].Flag), flagPriority(highlights[j].Flag)
		if pi != pj {
			return pi < pj
		}
		return highlights[i].Deadline.Before(highlights[j].Deadline)
	})

	return c.JSON(http.StatusOK, AccountingSummaryResponse{
		TotalPO:          int64(len(termins)),
		TotalTerminBelum: totalTerminBelum,
		TotalTerminSudah: totalTerminSudah,
		TotalLewat:       totalLewat,
		TotalMendekati:   totalMendekati,
		Highlights:       highlights,
	})
}

// ─── GET /accounting/po ────────────────────────────────────────────────────────

type AccountingPOItem struct {
	TrackingID             string                  `json:"trackingId"`
	NomorPenawaran         string                  `json:"nomorPenawaran"`
	PerusahaanName         string                  `json:"perusahaanName"`
	JenisPenawaran         []models.JenisPenawaran `json:"jenisPenawaran,omitempty"`
	TotalTermin            int                     `json:"totalTermin"`
	TerminSudah            int                     `json:"terminSudah"`
	PersentaseDibayar      float64                 `json:"persentaseDibayar"`
	EstimasiHarga          *float64                `json:"estimasiHarga,omitempty"`
	NominalDibayar         *float64                `json:"nominalDibayar,omitempty"`
	StatusPembayaran       string                  `json:"statusPembayaran"` // LUNAS | BELUM_LUNAS | OVERDUE
	TerminTerdekatNama     string                  `json:"terminTerdekatNama,omitempty"`
	TerminTerdekatDeadline *time.Time              `json:"terminTerdekatDeadline,omitempty"`
	TerminTerdekatFlag     string                  `json:"terminTerdekatFlag,omitempty"`
}

// sortAccountingPOItems mengurutkan seluruh hasil (sebelum dipotong per
// halaman) supaya urutannya konsisten lintas halaman pagination. Tanpa sortBy,
// urutannya tetap seperti semula: termin terdekat yang belum dibayar duluan,
// PO yang sudah lunas ditaruh paling belakang.
func sortAccountingPOItems(items []AccountingPOItem, sortBy, sortDir string) {
	desc := strings.EqualFold(sortDir, "desc")

	// compare mengembalikan -1/0/1 untuk urutan menaik.
	compare := func(a, b AccountingPOItem) int {
		switch strings.ToLower(sortBy) {
		case "nomorpenawaran":
			return strings.Compare(a.NomorPenawaran, b.NomorPenawaran)
		case "perusahaanname":
			return strings.Compare(a.PerusahaanName, b.PerusahaanName)
		case "status":
			return strings.Compare(a.StatusPembayaran, b.StatusPembayaran)
		case "persentasedibayar":
			switch {
			case a.PersentaseDibayar < b.PersentaseDibayar:
				return -1
			case a.PersentaseDibayar > b.PersentaseDibayar:
				return 1
			default:
				return 0
			}
		default: // "deadline" & nilai tak dikenal
			da, db := a.TerminTerdekatDeadline, b.TerminTerdekatDeadline
			switch {
			case da == nil && db == nil:
				return 0
			case da == nil:
				return 1
			case db == nil:
				return -1
			case da.Before(*db):
				return -1
			case db.Before(*da):
				return 1
			default:
				return 0
			}
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]

		// PO yang sudah lunas tidak punya termin terdekat. Saat diurutkan by
		// deadline, baris begini selalu ditaruh paling belakang — termasuk di
		// arah descending — supaya tidak menutupi termin yang masih berjalan.
		if isDeadlineSort(sortBy) {
			ea, eb := a.TerminTerdekatDeadline == nil, b.TerminTerdekatDeadline == nil
			if ea != eb {
				return eb
			}
		}

		result := compare(a, b)
		if desc {
			return result > 0
		}
		return result < 0
	})
}

// isDeadlineSort true untuk sortBy kosong (urutan bawaan) maupun kunci yang
// memang mengurutkan berdasarkan tenggat termin terdekat.
func isDeadlineSort(sortBy string) bool {
	switch strings.ToLower(sortBy) {
	case "nomorpenawaran", "perusahaanname", "status", "persentasedibayar":
		return false
	default:
		return true
	}
}

func GetAccountingPOList(c echo.Context) error {
	roleStr, divisiStr, ok := getAccountingDashboardClaims(c)
	if !ok || !isAccountingDashboardAuthorized(roleStr, divisiStr) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Tidak punya akses ke dashboard Accounting."})
	}

	page := max(1, toInt(c.QueryParam("page"), 1))
	limit := max(1, toInt(c.QueryParam("limit"), 20))
	search := strings.TrimSpace(c.QueryParam("search"))
	statusFilter := c.QueryParam("status") // "" | LUNAS | BELUM_LUNAS | OVERDUE
	// flagFilter dipakai shortcut card "Mendekati Tenggat": PO yang termin
	// terdekatnya jatuh tempo dalam 2 minggu ke depan tapi belum lewat.
	flagFilter := strings.ToUpper(c.QueryParam("flag")) // "" | MENDEKATI | LEWAT

	query := config.DB.Model(&models.TerminPembayaran{})
	if search != "" {
		like := "%" + search + "%"
		query = query.
			Joins(`JOIN "TrackingPenawaran" ON "TrackingPenawaran".id = "TerminPembayaran".tracking_penawaran_id`).
			Where(`"TrackingPenawaran".nomor_penawaran ILIKE ? OR "TrackingPenawaran".customer_name ILIKE ?`, like, like)
	}

	var termins []models.TerminPembayaran
	if err := query.
		Preload("Items").
		Preload("TrackingPenawaran.Perusahaan").
		Preload("TrackingPenawaran.PenyusunanBoQ").
		Find(&termins).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Gagal mengambil data accounting."})
	}

	now := time.Now()
	oneWeek := now.Add(7 * 24 * time.Hour)
	twoWeeks := now.Add(14 * 24 * time.Hour)

	items := make([]AccountingPOItem, 0, len(termins))
	for _, termin := range termins {
		var estimasi *float64
		if termin.TrackingPenawaran.PenyusunanBoQ != nil {
			estimasi = termin.TrackingPenawaran.PenyusunanBoQ.EstimasiHarga
		}

		item := AccountingPOItem{
			TrackingID:     termin.TrackingPenawaranID,
			NomorPenawaran: termin.TrackingPenawaran.NomorPenawaran,
			PerusahaanName: termin.TrackingPenawaran.Perusahaan.Nama,
			JenisPenawaran: termin.TrackingPenawaran.JenisPenawaran,
			TotalTermin:    len(termin.Items),
			EstimasiHarga:  estimasi,
		}

		persentaseDibayar := 0.0
		terlambat := false
		var nearestDeadline *time.Time
		var nearestNama, nearestFlag string

		for _, it := range termin.Items {
			if it.SudahDibayar {
				item.TerminSudah++
				persentaseDibayar += it.Persentase
				continue
			}

			flag := hitungFlag(it.Deadline, it.SudahDibayar, now, oneWeek, twoWeeks)
			if flag == "LEWAT" {
				terlambat = true
			}

			if it.Deadline != nil && (nearestDeadline == nil || it.Deadline.Before(*nearestDeadline)) {
				nearestDeadline = it.Deadline
				nearestNama = it.NamaTermin
				nearestFlag = flag
			}
		}

		item.PersentaseDibayar = persentaseDibayar
		if estimasi != nil {
			n := *estimasi * persentaseDibayar / 100
			item.NominalDibayar = &n
		}
		item.TerminTerdekatNama = nearestNama
		item.TerminTerdekatDeadline = nearestDeadline
		item.TerminTerdekatFlag = nearestFlag

		switch {
		case item.TotalTermin > 0 && item.TerminSudah == item.TotalTermin:
			item.StatusPembayaran = "LUNAS"
		case terlambat:
			item.StatusPembayaran = "OVERDUE"
		default:
			item.StatusPembayaran = "BELUM_LUNAS"
		}

		if statusFilter != "" && item.StatusPembayaran != statusFilter {
			continue
		}

		switch flagFilter {
		case "MENDEKATI":
			if item.TerminTerdekatFlag != "1_MINGGU" && item.TerminTerdekatFlag != "2_MINGGU" {
				continue
			}
		case "LEWAT":
			if item.TerminTerdekatFlag != "LEWAT" {
				continue
			}
		}

		items = append(items, item)
	}

	sortAccountingPOItems(items, c.QueryParam("sortBy"), c.QueryParam("sortDir"))

	total := len(items)
	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	offset := (page - 1) * limit
	end := offset + limit
	if offset > total {
		offset = total
	}
	if end > total {
		end = total
	}
	paged := items[offset:end]

	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": paged,
		"meta": PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      int64(total),
			TotalPages: totalPages,
		},
	})
}
