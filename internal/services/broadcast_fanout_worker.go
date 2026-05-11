package services

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"taxi-mvp/internal/config"
)

// RunBroadcastFanoutWorker delivers published broadcasts to all rider Telegram users (users.role='rider'),
// idempotent via broadcast_telegram_deliveries.
func RunBroadcastFanoutWorker(ctx context.Context, db *sql.DB, riderBot *tgbotapi.BotAPI, _ *config.Config) {
	if db == nil || riderBot == nil {
		return
	}
	// Conservative rate: 10 msg/s with small bursts.
	tick := time.NewTicker(120 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			deliverOne(ctx, db, riderBot)
		}
	}
}

func deliverOne(ctx context.Context, db *sql.DB, bot *tgbotapi.BotAPI) {
	// Pick one undelivered (broadcast_id, chat_id) pair.
	var (
		broadcastID string
		chatID      int64
		title       sql.NullString
		body        string
		mediaURL    sql.NullString
		mediaType   sql.NullString
	)
	err := db.QueryRowContext(ctx, `
		WITH candidates AS (
			SELECT b.id AS broadcast_id, u.telegram_id AS chat_id
			FROM broadcast_posts b
			JOIN users u ON u.role = 'rider' AND u.telegram_id != 0
			LEFT JOIN broadcast_telegram_deliveries d
			       ON d.broadcast_id = b.id AND d.chat_id = u.telegram_id
			WHERE b.status = 'published'
			  AND COALESCE(TRIM(b.body), '') != ''
			  AND COALESCE(b.audience, 'all_riders') = 'all_riders'
			  AND d.broadcast_id IS NULL
			ORDER BY datetime(b.created_at) DESC, b.id DESC, u.id ASC
			LIMIT 1
		)
		SELECT c.broadcast_id,
		       c.chat_id,
		       b.title,
		       b.body,
		       b.cloudinary_secure_url,
		       b.media_type
		FROM candidates c
		JOIN broadcast_posts b ON b.id = c.broadcast_id
		LIMIT 1
	`,).Scan(&broadcastID, &chatID, &title, &body, &mediaURL, &mediaType)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("broadcast fanout: pick: %v", err)
		}
		return
	}

	body = strings.TrimSpace(body)
	if body == "" || broadcastID == "" || chatID == 0 {
		return
	}

	// Telegram caption limit is ~1024; keep it safe.
	const captionMaxRunes = 900
	caption := truncateRunes(body, captionMaxRunes)

	var sendErr error
	mt := strings.ToLower(strings.TrimSpace(mediaType.String))
	if mediaURL.Valid && strings.TrimSpace(mediaURL.String) != "" && mt == "image" {
		msg := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(strings.TrimSpace(mediaURL.String)))
		msg.Caption = caption
		_, sendErr = bot.Send(msg)
	} else if mediaURL.Valid && strings.TrimSpace(mediaURL.String) != "" && mt == "video" {
		msg := tgbotapi.NewVideo(chatID, tgbotapi.FileURL(strings.TrimSpace(mediaURL.String)))
		msg.Caption = caption
		_, sendErr = bot.Send(msg)
	} else {
		text := caption
		if title.Valid && strings.TrimSpace(title.String) != "" {
			text = strings.TrimSpace(title.String) + "\n\n" + caption
		}
		_, sendErr = bot.Send(tgbotapi.NewMessage(chatID, text))
	}
	if sendErr != nil {
		// On 429 etc, just retry later (do not mark delivered).
		log.Printf("broadcast fanout: send broadcast_id=%s chat_id=%d: %v", broadcastID, chatID, sendErr)
		return
	}

	// Mark delivered (idempotent primary key).
	_, _ = db.ExecContext(ctx, `
		INSERT OR IGNORE INTO broadcast_telegram_deliveries (broadcast_id, chat_id, delivered_at)
		VALUES (?1, ?2, datetime('now'))`, broadcastID, chatID)
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max])) + "…"
}

