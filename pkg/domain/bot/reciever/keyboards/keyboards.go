package keyboards

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

type Keyboard struct {
	Name    string
	Buttons tgbotapi.ReplyKeyboardMarkup
}
