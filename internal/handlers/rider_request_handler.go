package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"taxi-mvp/internal/auth"
	"taxi-mvp/internal/config"
	"taxi-mvp/internal/services"
)

// RiderRequestDeps wires native rider ride-request HTTP handlers (Bearer auth).
type RiderRequestDeps struct {
	DB           *sql.DB
	Cfg          *config.Config
	RiderAuthSvc *services.RiderAuthService
	RiderReqSvc  *services.RiderRequestAppService
}

type riderCreateRequestBody struct {
	PickupLat       float64 `json:"pickup_lat"`
	PickupLng       float64 `json:"pickup_lng"`
	ClientRequestID string  `json:"client_request_id"`
}

type riderAppDestinationBody struct {
	DropLat  float64 `json:"drop_lat"`
	DropLng  float64 `json:"drop_lng"`
	DropName string  `json:"drop_name"`
}

// RegisterRiderRequestRoutes mounts Bearer-authenticated ride request routes
// under /v1/rider/requests* (same DB + dispatch path as Telegram rider bot).
func RegisterRiderRequestRoutes(r *gin.Engine, deps RiderRequestDeps) {
	if r == nil || deps.DB == nil || deps.RiderAuthSvc == nil || deps.RiderReqSvc == nil {
		return
	}
	bearer := RequireRiderBearerAuth(deps.RiderAuthSvc, deps.DB)
	g := r.Group("/v1/rider")
	g.Use(bearer)
	g.POST("/requests", riderAppCreateRequest(deps))
	g.POST("/requests/:id/destination", riderAppSetDestination(deps))
	g.POST("/requests/:id/confirm", riderAppConfirmRequest(deps))
}

func riderUserID(c *gin.Context) (int64, bool) {
	u := auth.UserFromContext(c.Request.Context())
	if u == nil || u.UserID <= 0 {
		writeRiderAPIError(c, http.StatusUnauthorized, "invalid_token", "Kirish talab qilinadi.")
		return 0, false
	}
	return u.UserID, true
}

func riderAppCreateRequest(deps RiderRequestDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := riderUserID(c)
		if !ok {
			return
		}
		var body riderCreateRequestBody
		if err := c.ShouldBindJSON(&body); err != nil {
			writeRiderAPIError(c, http.StatusBadRequest, "invalid_body", "Yuborilgan ma‘lumot noto‘g‘ri.")
			return
		}
		reqID, err := deps.RiderReqSvc.CreatePickupRequest(c.Request.Context(), uid, body.PickupLat, body.PickupLng, strings.TrimSpace(body.ClientRequestID))
		if err != nil {
			mapRiderRequestError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"request_id": reqID})
	}
}

func riderAppSetDestination(deps RiderRequestDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := riderUserID(c)
		if !ok {
			return
		}
		id := strings.TrimSpace(c.Param("id"))
		var body riderAppDestinationBody
		if err := c.ShouldBindJSON(&body); err != nil {
			writeRiderAPIError(c, http.StatusBadRequest, "invalid_body", "Yuborilgan ma‘lumot noto‘g‘ri.")
			return
		}
		est, err := deps.RiderReqSvc.SetDestination(c.Request.Context(), uid, id, body.DropLat, body.DropLng, body.DropName)
		if err != nil {
			mapRiderRequestError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "estimated_price": est})
	}
}

func riderAppConfirmRequest(deps RiderRequestDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := riderUserID(c)
		if !ok {
			return
		}
		id := strings.TrimSpace(c.Param("id"))
		if err := deps.RiderReqSvc.ConfirmRequest(c.Request.Context(), uid, id); err != nil {
			mapRiderRequestError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func mapRiderRequestError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrRiderRequestLegalRequired):
		writeRiderAPIError(c, http.StatusForbidden, "legal_required", "Iltimos, avval foydalanuvchi shartlari va maxfiylik siyosatini qabul qiling.")
	case errors.Is(err, services.ErrRiderRequestPhoneRequired):
		writeRiderAPIError(c, http.StatusForbidden, "phone_required", "Telefon raqamingiz profilda yo‘q. Iltimos, rider botda kontakt yuboring.")
	case errors.Is(err, services.ErrRiderRequestAbuseBlocked):
		writeRiderAPIError(c, http.StatusForbidden, "abuse_blocked", "Buyurtma vaqtincha cheklangan. Keyinroq qayta urinib ko‘ring.")
	case errors.Is(err, services.ErrRiderRequestDuplicatePending):
		writeRiderAPIError(c, http.StatusConflict, "duplicate_pending", "Sizda allaqachon faol so‘rov bor. Haydovchi topilguncha yoki bekor qilinguncha kuting.")
	case errors.Is(err, services.ErrRiderRequestNotFound):
		writeRiderAPIError(c, http.StatusNotFound, "not_found", "So‘rov topilmadi.")
	case errors.Is(err, services.ErrRiderRequestConflictState):
		writeRiderAPIError(c, http.StatusConflict, "conflict", "So‘rov holati noto‘g‘ri yoki muddati tugagan.")
	case errors.Is(err, services.ErrRiderRequestInvalidCoords):
		writeRiderAPIError(c, http.StatusBadRequest, "invalid_coordinates", "Koordinatalar noto‘g‘ri.")
	case errors.Is(err, services.ErrRiderRequestMatchUnavailable):
		writeRiderAPIError(c, http.StatusServiceUnavailable, "service_unavailable", "Xizmat vaqtincha ishlamayapti.")
	default:
		writeRiderAPIError(c, http.StatusInternalServerError, "internal_error", "Texnik xatolik.")
	}
}
