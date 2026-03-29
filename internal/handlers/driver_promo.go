package handlers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"taxi-mvp/internal/accounting"
	"taxi-mvp/internal/auth"
)

// DriverPromoProgram returns current promo program progress and promo_balance for the authenticated driver.
func DriverPromoProgram(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := auth.UserFromContext(c.Request.Context())
		if u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		st, err := accounting.GetDriverPromoProgramStatus(c.Request.Context(), db, u.UserID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load promo status"})
			return
		}
		c.JSON(http.StatusOK, st)
	}
}
