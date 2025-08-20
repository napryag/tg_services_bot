package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/napryag/tg_services_bot/pkg/domain/bot/reciever/config"
	"github.com/napryag/tg_services_bot/pkg/utils/errs"
	"github.com/rs/zerolog"
)

func main() {
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Err(errs.New("failed to load config").Wrap(err)).Msg("config init")
		return
	}

	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		logger.Error().Err(err).Msg("create bot api")
		return
	}

	bot.Debug = false

	logger.Info().Str("bot", bot.Self.UserName).Msg("authorized")

	// Контекст, завершающийся по SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 10
	updates := bot.GetUpdatesChan(u)

	// Горутина для корректного завершения
	go func() {
		<-ctx.Done()
		logger.Info().Msg("shutting down bot")
		// Останавливаем лонг-поллинг -> канал updates закроется, цикл ниже завершится
		bot.StopReceivingUpdates()
	}()

	for update := range updates {
		if update.Message != nil && update.Message.IsCommand() {
			switch update.Message.Command() {
			case "start":
				if _, err := bot.Request(tgbotapi.NewDeleteMessage(update.Message.Chat.ID, update.Message.MessageID)); err != nil {
					logger.Warn().Err(err).Msg("delete /start failed")
				}
				msg := tgbotapi.NewPhoto(update.Message.Chat.ID, tgbotapi.FilePath("pictures/logo.png"))
				msg.Caption = "Вот картинка с подписью <b>жирным</b> и эмодзи 😺"
				msg.ParseMode = "HTML"
				keyboard := tgbotapi.NewReplyKeyboard(
					tgbotapi.NewKeyboardButtonRow(
						tgbotapi.NewKeyboardButton("Запись"),
						tgbotapi.NewKeyboardButton("Мои записи"),
					),
					tgbotapi.NewKeyboardButtonRow(
						tgbotapi.NewKeyboardButton("Помощь"),
					),
				)
				keyboard.ResizeKeyboard = true
				keyboard.OneTimeKeyboard = false

				msg.ReplyMarkup = keyboard
				if _, err := bot.Send(msg); err != nil {
					logger.Error().Err(err).Msg("send photo error")
				}

			}
		} else if update.Message != nil { // If we got a message
			logger.Info().
				Str("user", update.Message.From.UserName).
				Str("text", update.Message.Text).
				Msg("incoming")

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, update.Message.Text)
			msg.ReplyToMessageID = update.Message.MessageID

			if _, err := bot.Send(msg); err != nil {
				logger.Error().Err(err).Msg("send echo error")
			}
		} else {
			logger.Warn().Msg("unsupported update type")
		}
	}

	logger.Info().Msg("bot stopped")
}
