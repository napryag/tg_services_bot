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
	"github.com/napryag/tg_services_bot/pkg/domain/bot/receiver"
	"github.com/napryag/tg_services_bot/pkg/domain/bot/receiver/config"
	"github.com/napryag/tg_services_bot/pkg/utils/errs"
	"github.com/rs/zerolog"
)

func main() {

	// 1) Контекст
	ctx := context.Background()

	// 2) Логгер
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()

	// 3) Загружаем конфиг
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

	store := receiver.NewStore()

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

			// если это /start — обработали и уходим к след. апдейту
			if handled := handleStartCommand(m, sess, bot, logger, update); handled {
				continue
			}

			// Любой произвольный текст — удаляем (если возможно) и напоминаем
			_, _ = bot.Request(tgbotapi.NewDeleteMessage(m.Chat.ID, m.MessageID))

			remind := tgbotapi.NewMessage(m.Chat.ID, "Пожалуйста, используйте кнопки 👆")
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
			case data == receiver.CbStart:
				sess.Go(receiver.StateMain)
			case data == receiver.CbBook:
				sess.Go(receiver.StateBookService)
			case data == receiver.CbMy:
				sess.Go(receiver.StateMy)
			case data == receiver.CbHelp:
				sess.Go(receiver.StateHelp)
			case data == receiver.CbBack:
				sess.Back()

			case strings.HasPrefix(data, receiver.PSvc):
				val, _ := receiver.Is(data, receiver.PSvc)
				sess.Booking.Service = val
				sess.Go(receiver.StateBookMaster)

			case strings.HasPrefix(data, receiver.PM):
				val, _ := receiver.Is(data, receiver.PM)
				sess.Booking.Master = val
				sess.Go(receiver.StateBookDate)

			case strings.HasPrefix(data, receiver.PD):
				val, _ := receiver.Is(data, receiver.PD)
				sess.Booking.Date = val
				sess.Go(receiver.StateBookTime)

			case strings.HasPrefix(data, receiver.PT):
				val, _ := receiver.Is(data, receiver.PT)
				sess.Booking.Time = val
				sess.Go(receiver.StateBookConfirm)

			case data == receiver.CbOk:
				// TODO: здесь сохраняем запись в вашу БД/сервис
				// booking := sess.Booking
				// err := appointmentsService.Create(booking)
				// ...
				text := fmt.Sprintf("Готово! Вы записаны: %s, %s, %s, %s.",
					receiver.Title(sess.Booking.Service), receiver.Title(sess.Booking.Master),
					receiver.HumanDate(sess.Booking.Date), sess.Booking.Time,
				)
				sess.ResetFlow() // возвращаемся в главное меню

				edit := tgbotapi.NewEditMessageTextAndMarkup(
					cq.Message.Chat.ID, cq.Message.MessageID, text, receiver.MainMenu(),
				)
				_, _ = bot.Send(edit)

				// Гасим "часики"
				_, _ = bot.Request(tgbotapi.NewCallback(cq.ID, ""))
				continue
			}

			// Рендерим текущий экран (редактируем то же сообщение)
			capt, rep := receiver.NewEditMessageCaptionAndMarkup(
				cq.Message.Chat.ID, cq.Message.MessageID, receiver.RenderText(sess), receiver.RenderKeyboard(sess),
			)
			if _, err := bot.Send(capt); err != nil {
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

func handleStartCommand(
	m *tgbotapi.Message,
	sess *receiver.Session,
	bot *tgbotapi.BotAPI,
	logger zerolog.Logger,
	update tgbotapi.Update,
) bool {
	if m.IsCommand() && m.Command() == "start" {
		sess.ResetFlow()
		sess.State = receiver.StateStart
		if _, err := bot.Request(tgbotapi.NewDeleteMessage(update.Message.Chat.ID, update.Message.MessageID)); err != nil {
			logger.Warn().Err(err).Msg("delete /start failed")
		}
		msg := tgbotapi.NewPhoto(update.Message.Chat.ID, tgbotapi.FilePath("pictures/logo.png"))
		msg.Caption = fmt.Sprintf("<b>Приветствую %s!\n"+
			"Данный чат-бот поможет Вам записаться на услуги барбера. Здесь вы можете отслеживать свои записи и т.д.\n"+
			"Для того, чтобы начать работу с нашим ботом нажмите НАЧАТЬ</b>😺", m.From.FirstName)
		msg.ParseMode = "HTML"
		msg.ReplyMarkup = receiver.RenderKeyboard(sess)
		if _, err := bot.Send(msg); err != nil {
			logger.Printf("send start menu error: %v", err)
		}
	}
	return false
}
