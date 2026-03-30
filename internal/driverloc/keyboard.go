package driverloc

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

// ReplyKeyboardButtonShareLiveLocation returns a plain reply-keyboard label: tap sends text only.
// The bot responds with the full live-location guide every time; it does not use request_location
// (no map picker / no implicit location actions).
func ReplyKeyboardButtonShareLiveLocation() tgbotapi.KeyboardButton {
	return tgbotapi.NewKeyboardButton(BtnShareLiveLocation)
}
