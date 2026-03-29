package services

import (
	"context"
	"database/sql"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// RunOnlineBonusWorker is retained for startup wiring compatibility. The legacy online-time promo program
// (hourly promo accrual while live) is disabled; promo is granted only via signup + first 3 trips (see accounting/driver_promo.go).
func RunOnlineBonusWorker(ctx context.Context, db *sql.DB, driverBot *tgbotapi.BotAPI) {
	_ = db
	_ = driverBot
	<-ctx.Done()
}
