package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/napryag/tg_services_bot/pkg/domain/bot/reciever"
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

	store := reciever.NewStore()

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
		if m := update.Message; m != nil {
			userID := m.From.ID
			sess := store.Get(userID)

			if m.IsCommand() && m.Command() == "start" {
				sess.ResetFlow()
				sess.State = reciever.StateStart
				if _, err := bot.Request(tgbotapi.NewDeleteMessage(update.Message.Chat.ID, update.Message.MessageID)); err != nil {
					logger.Warn().Err(err).Msg("delete /start failed")
				}
				msg := tgbotapi.NewPhoto(update.Message.Chat.ID, tgbotapi.FilePath("pictures/logo.png"))
				msg.Caption = fmt.Sprintf("<b>Приветствую %s!\nДанный чат-бот поможет Вам записаться на услуги барбера. Здесь вы можете отслеживать свои записи и т.д.\nДля того, чтобы начать работу с нашим ботом нажмите НАЧАТЬ</b>😺", m.From.FirstName)
				msg.ParseMode = "HTML"
				msg.ReplyMarkup = reciever.RenderKeyboard(sess)
				if _, err := bot.Send(msg); err != nil {
					logger.Printf("send start menu error: %v", err)
				}
				continue
			}

			// Любой произвольный текст — удаляем (если возможно) и напоминаем
			_, _ = bot.Request(tgbotapi.NewDeleteMessage(m.Chat.ID, m.MessageID))

			remind := tgbotapi.NewMessage(m.Chat.ID, "Пожалуйста, используйте кнопки 👇")
			remind.ReplyMarkup = reciever.RenderKeyboard(sess)
			sent, _ := bot.Send(remind)
			go func(chatID int64, mid int) {
				time.Sleep(5 * time.Second)
				_, _ = bot.Request(tgbotapi.NewDeleteMessage(chatID, mid))
			}(sent.Chat.ID, sent.MessageID)
			continue
		}
		// Нажатия на inline-кнопки
		if cq := update.CallbackQuery; cq != nil {
			userID := cq.From.ID
			sess := store.Get(userID)
			data := cq.Data

			switch {
			case data == reciever.CbStart:
				sess.Go(reciever.StateMain)
			case data == reciever.CbBook:
				sess.Go(reciever.StateBookService)
			case data == reciever.CbMy:
				sess.Go(reciever.StateMy)
			case data == reciever.CbHelp:
				sess.Go(reciever.StateHelp)
			case data == reciever.CbBack:
				sess.Back()

			case strings.HasPrefix(data, reciever.PSvc):
				val, _ := reciever.Is(data, reciever.PSvc)
				sess.Booking.Service = val
				sess.Go(reciever.StateBookMaster)

			case strings.HasPrefix(data, reciever.PM):
				val, _ := reciever.Is(data, reciever.PM)
				sess.Booking.Master = val
				sess.Go(reciever.StateBookDate)

			case strings.HasPrefix(data, reciever.PD):
				val, _ := reciever.Is(data, reciever.PD)
				sess.Booking.Date = val
				sess.Go(reciever.StateBookTime)

			case strings.HasPrefix(data, reciever.PT):
				val, _ := reciever.Is(data, reciever.PT)
				sess.Booking.Time = val
				sess.Go(reciever.StateBookConfirm)

			case data == reciever.CbOk:
				// TODO: здесь сохраняем запись в вашу БД/сервис
				// booking := sess.Booking
				// err := appointmentsService.Create(booking)
				// ...
				text := fmt.Sprintf("Готово! Вы записаны: %s, %s, %s, %s.",
					reciever.Title(sess.Booking.Service), reciever.Title(sess.Booking.Master),
					reciever.HumanDate(sess.Booking.Date), sess.Booking.Time,
				)
				sess.ResetFlow() // возвращаемся в главное меню

				edit := tgbotapi.NewEditMessageTextAndMarkup(
					cq.Message.Chat.ID, cq.Message.MessageID, text, reciever.MainMenu(),
				)
				_, _ = bot.Send(edit)

				// Гасим "часики"
				_, _ = bot.Request(tgbotapi.NewCallback(cq.ID, ""))
				continue
			}

			// Рендерим текущий экран (редактируем то же сообщение)
			cap, rep := reciever.NewEditMessageCaptionAndMarkup(
				cq.Message.Chat.ID, cq.Message.MessageID, reciever.RenderText(sess), reciever.RenderKeyboard(sess),
			)
			if _, err := bot.Send(cap); err != nil {
				log.Printf("cap error: %v", err)
			}
			if _, err := bot.Send(rep); err != nil {
				log.Printf("rep error: %v", err)
			}
			_, _ = bot.Request(tgbotapi.NewCallback(cq.ID, ""))
		}
	}
	logger.Info().Msg("bot stopped")
}
