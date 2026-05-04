// Package riderauth contains the small Telegram-side helpers used by the
// rider native-auth (phone + OTP) flow.
//
// It is a leaf package: it imports only the Telegram client library and is
// safe to depend on from internal/services without creating an import cycle
// (the parent internal/bot/rider package itself already imports
// internal/services for the rider Telegram bot's MatchService / TripService
// dependencies).
//
// Conceptually this file is the "rider login notifier" referenced in the
// spec — see docs/AUTH.md and the rider auth service. It deliberately does
// not log the OTP plaintext.
package riderauth

import (
	"errors"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// LoginCodeSender is the narrow surface of *tgbotapi.BotAPI we need to send
// a login code. Defined as an interface so service tests can swap in a fake.
type LoginCodeSender interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
}

// LoginCodeSendOutcome describes how the Telegram delivery went.
type LoginCodeSendOutcome int

const (
	// LoginCodeSent: delivered successfully.
	LoginCodeSent LoginCodeSendOutcome = iota
	// LoginCodeBotBlocked: Telegram returned 403 (the rider blocked the bot
	// or never started it). Caller should respond 409 bot_blocked.
	LoginCodeBotBlocked
	// LoginCodeFailed: any other error after a single transient retry.
	LoginCodeFailed
)

// RenderLoginCodeMessage builds the HTML body sent to the rider. It is
// extracted so tests can assert on it without touching tgbotapi.
//
// It uses Telegram's HTML parse mode. The plaintext code itself is the only
// dynamic part, and it is wrapped in <code> so the rider can long-press to
// copy it on mobile.
func RenderLoginCodeMessage(code string) string {
	var b strings.Builder
	b.WriteString("<b>YettiQanot Rider</b>\n")
	b.WriteString("Kirish kodingiz: <code>")
	b.WriteString(code)
	b.WriteString("</code>\n")
	b.WriteString("Bu kod 5 daqiqada eskiradi. Bu kodni hech kimga aytmang.")
	return b.String()
}

// SendLoginCode sends a login OTP to the rider's Telegram chat using the
// rider bot. It performs a single short retry on transient errors (network /
// 5xx). 403 ("bot was blocked by the user") is reported separately so the
// caller can return a 409 to the rider app with an Uzbek hint.
//
// Returns the outcome plus the underlying Telegram error (if any) so the
// caller can include the description in its audit log. The plaintext code
// is held only in this function's stack frame and is never logged here.
func SendLoginCode(sender LoginCodeSender, telegramID int64, code string) (LoginCodeSendOutcome, error) {
	if sender == nil || telegramID == 0 || code == "" {
		return LoginCodeFailed, errInvalidArgs
	}
	msg := tgbotapi.NewMessage(telegramID, RenderLoginCodeMessage(code))
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true

	_, err := sender.Send(msg)
	if err == nil {
		return LoginCodeSent, nil
	}
	if isTelegramBotBlocked(err) {
		return LoginCodeBotBlocked, err
	}
	if !isTelegramTransient(err) {
		return LoginCodeFailed, err
	}
	// One short retry on transient 5xx / network errors, then fail.
	time.Sleep(300 * time.Millisecond)
	_, err2 := sender.Send(msg)
	if err2 == nil {
		return LoginCodeSent, nil
	}
	if isTelegramBotBlocked(err2) {
		return LoginCodeBotBlocked, err2
	}
	return LoginCodeFailed, err2
}

// errInvalidArgs is returned by SendLoginCode when sender / telegramID /
// code are missing or zero. Kept as a package-level value so the service
// log line can include a stable string instead of "<nil>".
var errInvalidArgs = invalidArgsError{}

type invalidArgsError struct{}

func (invalidArgsError) Error() string { return "rider auth: invalid send args" }

// isTelegramBotBlocked returns true if the Telegram API rejected the send
// because the rider blocked the bot or never started a conversation.
func isTelegramBotBlocked(err error) bool {
	if err == nil {
		return false
	}
	var tgErr *tgbotapi.Error
	if errors.As(err, &tgErr) {
		if tgErr.Code == 403 {
			return true
		}
	}
	// Library does not always wrap into *Error; fall back to substring check
	// on the standard Telegram description so we don't miss the case.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "bot was blocked by the user") ||
		strings.Contains(msg, "user is deactivated") ||
		strings.Contains(msg, "chat not found") ||
		strings.Contains(msg, "forbidden")
}

// isTelegramTransient reports whether err looks like a 5xx / network blip
// that we should retry once.
func isTelegramTransient(err error) bool {
	if err == nil {
		return false
	}
	var tgErr *tgbotapi.Error
	if errors.As(err, &tgErr) {
		// 4xx (other than 403 handled above) is not transient.
		return tgErr.Code >= 500 && tgErr.Code <= 599
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporarily unavailable") ||
		strings.Contains(msg, "internal server error") ||
		strings.Contains(msg, "bad gateway") ||
		strings.Contains(msg, "gateway timeout") ||
		strings.Contains(msg, "service unavailable") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "eof")
}
