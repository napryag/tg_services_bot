package reciever

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ---------- FSM ----------

type State int

const (
	StateStart State = iota
	StateMain
	StateBookService
	StateBookMaster
	StateBookDate
	StateBookTime
	StateBookConfirm
	StateMy
	StateHelp
)

type BookingData struct {
	Service string
	Master  string
	Date    string // YYYY-MM-DD
	Time    string // HH:MM
}

type Session struct {
	State   State
	history []State
	Booking BookingData
}

func (s *Session) Go(to State) {
	s.history = append(s.history, s.State)
	s.State = to
}

func (s *Session) Back() {
	if n := len(s.history); n > 0 {
		s.State = s.history[n-1]
		s.history = s.history[:n-1]
	} else {
		s.State = StateMain
	}
}

func (s *Session) ResetFlow() {
	s.State = StateMain
	s.history = s.history[:0]
	s.Booking = BookingData{}
}

// ---------- Session store (in-memory, потокобезопасно) ----------

type Store struct {
	mu sync.RWMutex
	m  map[int64]*Session
}

func NewStore() *Store {
	return &Store{m: make(map[int64]*Session)}
}

func (s *Store) Get(userID int64) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.m[userID]; ok {
		return sess
	}
	se := &Session{State: StateMain}
	s.m[userID] = se
	return se
}

// ---------- Callback keys ----------

const (
	CbStart = "start"
	CbMain  = "main"
	CbBook  = "book"
	CbMy    = "my"
	CbHelp  = "help"
	CbBack  = "back"
	CbOk    = "confirm"

	PSvc = "svc:" // svc:haircut
	PM   = "m:"   // m:john
	PD   = "d:"   // d:2025-08-20
	PT   = "t:"   // t:10:30
)

func Is(k, prefix string) (string, bool) {
	if strings.HasPrefix(k, prefix) {
		return strings.TrimPrefix(k, prefix), true
	}
	return "", false
}

// ---------- UI builders ----------
func StartMenu() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("НАЧАТЬ", CbStart)),
	)
}

func MainMenu() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("💈 Запись", CbBook)),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("📅 Мои записи", CbMy)),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("❓ Помощь", CbHelp)),
	)
}

func ServiceMenu() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Стрижка", PSvc+"haircut"),
			tgbotapi.NewInlineKeyboardButtonData("Бритьё", PSvc+"shave"),
		),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", CbBack)),
	)
}

func MastersMenu() tgbotapi.InlineKeyboardMarkup {
	// заглушка: подставьте ваших мастеров
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Андрей", PM+"andrey"),
			tgbotapi.NewInlineKeyboardButtonData("Мария", PM+"maria"),
		),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", CbBack)),
	)
}

func DateMenu() tgbotapi.InlineKeyboardMarkup {
	now := time.Now()
	d1 := now.AddDate(0, 0, 0).Format("2006-01-02")
	d2 := now.AddDate(0, 0, 1).Format("2006-01-02")
	d3 := now.AddDate(0, 0, 2).Format("2006-01-02")
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(HumanDate(d1), PD+d1),
			tgbotapi.NewInlineKeyboardButtonData(HumanDate(d2), PD+d2),
			tgbotapi.NewInlineKeyboardButtonData(HumanDate(d3), PD+d3),
		),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", CbBack)),
	)
}

func TimeMenu() tgbotapi.InlineKeyboardMarkup {
	// заглушка; можно генерировать по расписанию
	slots := []string{"10:00", "12:00", "14:00", "16:00"}
	row := make([]tgbotapi.InlineKeyboardButton, 0, len(slots))
	for _, t := range slots {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(t, PT+t))
	}
	return tgbotapi.NewInlineKeyboardMarkup(
		row,
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", CbBack)),
	)
}

func ConfirmMenu() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("✅ Подтвердить", CbOk)),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", CbBack)),
	)
}

func HumanDate(iso string) string {
	t, _ := time.Parse("2006-01-02", iso)
	return t.Format("02.01 (Mon)")
}

// ---------- Rendering по состоянию ----------

func RenderText(sess *Session) string {
	switch sess.State {
	case StateMain:
		return "Выберите действие:"
	case StateBookService:
		return "Выберите услугу:"
	case StateBookMaster:
		return "Выберите мастера:"
	case StateBookDate:
		return "Выберите дату:"
	case StateBookTime:
		return "Выберите время:"
	case StateBookConfirm:
		return fmt.Sprintf(
			"Проверьте запись:\nУслуга: %s\nМастер: %s\nДата: %s\nВремя: %s",
			Title(sess.Booking.Service), Title(sess.Booking.Master),
			HumanDate(sess.Booking.Date), sess.Booking.Time,
		)
	case StateMy:
		return "Ваши записи (заглушка):\n— 21.08 14:00, Стрижка, Андрей"
	case StateHelp:
		return "Помощь (заглушка):\nНажмите «Запись», чтобы выбрать услугу и время."
	default:
		return "Меню"
	}
}

func RenderKeyboard(sess *Session) tgbotapi.InlineKeyboardMarkup {
	switch sess.State {
	case StateStart:
		return StartMenu()
	case StateMain:
		return MainMenu()
	case StateBookService:
		return ServiceMenu()
	case StateBookMaster:
		return MastersMenu()
	case StateBookDate:
		return DateMenu()
	case StateBookTime:
		return TimeMenu()
	case StateBookConfirm:
		return ConfirmMenu()
	case StateMy:
		// Покажем кнопку назад к главному
		return tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", CbBack)),
		)
	case StateHelp:
		return tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("⬅️ Назад", CbBack)),
		)
	default:
		return MainMenu()
	}
}

func NewEditMessageCaptionAndMarkup(chatID int64, messageID int, caption string, replyMarkup tgbotapi.InlineKeyboardMarkup) (tgbotapi.EditMessageCaptionConfig, tgbotapi.EditMessageReplyMarkupConfig) {
	cap := tgbotapi.NewEditMessageCaption(chatID, messageID, caption)
	rep := tgbotapi.NewEditMessageReplyMarkup(chatID, messageID, replyMarkup)
	return cap, rep
}

func Title(s string) string {
	switch s {
	case "haircut":
		return "Стрижка"
	case "shave":
		return "Бритьё"
	case "andrey":
		return "Андрей"
	case "maria":
		return "Мария"
	}
	return s
}
