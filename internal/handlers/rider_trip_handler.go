package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"taxi-mvp/internal/auth"
	"taxi-mvp/internal/domain"
	"taxi-mvp/internal/services"
)

// RiderTripDeps wires native rider trip HTTP handlers (Bearer auth).
type RiderTripDeps struct {
	DB           *sql.DB
	RiderAuthSvc *services.RiderAuthService
	TripSvc      *services.TripService
}

// RegisterRiderTripRoutes mounts Bearer-authenticated trip routes under /v1/rider/trips/*.
//
// This is intended for the native Flutter rider app, which does not have Telegram init_data.
func RegisterRiderTripRoutes(r *gin.Engine, deps RiderTripDeps) {
	if r == nil || deps.DB == nil || deps.RiderAuthSvc == nil || deps.TripSvc == nil {
		return
	}
	bearer := RequireRiderBearerAuth(deps.RiderAuthSvc, deps.DB)
	g := r.Group("/v1/rider")
	g.Use(bearer)

	// Preferred: no body, trip id in path.
	g.POST("/trips/:id/cancel", riderAppCancelTripByPath(deps))
	// Convenience: body { "trip_id": "..." }.
	g.POST("/trips/cancel", riderAppCancelTripByBody(deps))
}

type riderTripCancelBody struct {
	TripID string `json:"trip_id"`
}

func riderAppCancelTripByPath(deps RiderTripDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := riderUserID(c)
		if !ok {
			return
		}
		tripID := strings.TrimSpace(c.Param("id"))
		if tripID == "" {
			writeRiderAPIError(c, http.StatusBadRequest, "invalid_body", "Yuborilgan ma‘lumot noto‘g‘ri.")
			return
		}
		riderAppCancelTrip(c, deps, uid, tripID)
	}
}

func riderAppCancelTripByBody(deps RiderTripDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := riderUserID(c)
		if !ok {
			return
		}
		var body riderTripCancelBody
		if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.TripID) == "" {
			writeRiderAPIError(c, http.StatusBadRequest, "invalid_body", "Yuborilgan ma‘lumot noto‘g‘ri.")
			return
		}
		riderAppCancelTrip(c, deps, uid, strings.TrimSpace(body.TripID))
	}
}

func riderAppCancelTrip(c *gin.Context, deps RiderTripDeps, riderUserID int64, tripID string) {
	ctx := c.Request.Context()

	// Ensure the trip exists and belongs to the authenticated rider.
	ok, err := auth.AuthorizeTripAccess(ctx, deps.DB, riderUserID, tripID, domain.RoleRider)
	if err != nil {
		writeRiderAPIError(c, http.StatusInternalServerError, "internal_error", "Texnik xatolik.")
		return
	}
	if !ok {
		// Avoid leaking existence; treat as not_found (same style as rider requests).
		writeRiderAPIError(c, http.StatusNotFound, "not_found", "Safar topilmadi.")
		return
	}

	result, err := deps.TripSvc.CancelByRider(ctx, tripID, riderUserID)
	if err != nil {
		mapRiderTripCancelError(c, tripID, err)
		return
	}

	// Keep response stable and close to the non-v1 /trip/cancel/rider result,
	// but wrapped in v1 rider JSON conventions.
	if result == nil || result.Result == "noop" {
		c.JSON(http.StatusOK, gin.H{"ok": true, "result": "noop"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"trip_id": tripID,
		"status":  result.Status,
		"result":  result.Result,
	})
}

func mapRiderTripCancelError(c *gin.Context, tripID string, err error) {
	switch {
	case errors.Is(err, domain.ErrTripNotFound):
		writeRiderAPIError(c, http.StatusNotFound, "not_found", "Safar topilmadi.")
	case errors.Is(err, domain.ErrInvalidTransition),
		errors.Is(err, domain.ErrAlreadyFinished),
		errors.Is(err, domain.ErrAlreadyCancelled):
		writeRiderAPIError(c, http.StatusConflict, "conflict", "Safarni bekor qilib bo‘lmaydi.")
	default:
		writeRiderAPIError(c, http.StatusInternalServerError, "internal_error", "Texnik xatolik.")
	}
}

