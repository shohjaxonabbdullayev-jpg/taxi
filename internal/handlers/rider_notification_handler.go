package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"taxi-mvp/internal/services"
)

// RiderNotificationDeps wires GET /v1/rider/notifications (Bearer auth).
type RiderNotificationDeps struct {
	DB           *sql.DB
	RiderAuthSvc *services.RiderAuthService
}

// RegisterRiderNotificationRoutes mounts Bearer-authenticated notification list
// under /v1/rider/notifications for the native Flutter rider app.
func RegisterRiderNotificationRoutes(r *gin.Engine, deps RiderNotificationDeps) {
	if r == nil || deps.DB == nil || deps.RiderAuthSvc == nil {
		return
	}
	bearer := RequireRiderBearerAuth(deps.RiderAuthSvc, deps.DB)
	g := r.Group("/v1/rider")
	g.Use(bearer)
	g.GET("/notifications", riderAppListNotifications(deps))
}

func riderAppListNotifications(deps RiderNotificationDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := riderUserID(c)
		if !ok {
			return
		}
		ctx := c.Request.Context()

		limit := 50
		if s := strings.TrimSpace(c.Query("limit")); s != "" {
			if n, err := strconv.Atoi(s); err == nil {
				switch {
				case n < 1:
					limit = 50
				case n > 100:
					limit = 100
				default:
					limit = n
				}
			}
		}

		rows, err := deps.DB.QueryContext(ctx, `
			SELECT id, title, body, created_at
			FROM rider_app_notifications
			WHERE rider_user_id = ?1
			  AND TRIM(body) != ''
			ORDER BY datetime(created_at) DESC, id DESC
			LIMIT ?2`, uid, limit)
		if err != nil {
			writeRiderAPIError(c, http.StatusInternalServerError, "internal_error", "Texnik xatolik.")
			return
		}
		defer rows.Close()

		type item struct {
			ID        string `json:"id"`
			Title     string `json:"title,omitempty"`
			Body      string `json:"body"`
			CreatedAt string `json:"created_at"`
		}
		var list []item
		for rows.Next() {
			var id, body, createdAt string
			var title sql.NullString
			if err := rows.Scan(&id, &title, &body, &createdAt); err != nil {
				writeRiderAPIError(c, http.StatusInternalServerError, "internal_error", "Texnik xatolik.")
				return
			}
			body = strings.TrimSpace(body)
			if body == "" {
				continue
			}
			it := item{
				ID:        strings.TrimSpace(id),
				Body:      body,
				CreatedAt: normalizeRiderNotificationTime(createdAt),
			}
			if title.Valid {
				it.Title = strings.TrimSpace(title.String)
			}
			list = append(list, it)
		}
		if err := rows.Err(); err != nil {
			writeRiderAPIError(c, http.StatusInternalServerError, "internal_error", "Texnik xatolik.")
			return
		}
		if list == nil {
			list = []item{}
		}
		c.JSON(http.StatusOK, gin.H{"notifications": list})
	}
}

func normalizeRiderNotificationTime(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z07:00",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return s
}
