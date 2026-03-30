package driverloc

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

// ReplyKeyboardButtonShareLiveLocation returns a reply-keyboard button that opens Telegram’s
// location picker (request_location). Plain NewKeyboardButton(BtnShareLiveLocation) only sends
// text and does not request coordinates — use this for the driver “share location” flow.
func ReplyKeyboardButtonShareLiveLocation() tgbotapi.KeyboardButton {
	return tgbotapi.NewKeyboardButtonLocation(BtnShareLiveLocation)
}
