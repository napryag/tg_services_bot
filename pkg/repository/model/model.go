package model

import (
	"context"
	"time"
)

type User struct {
	ID        int64
	TgUserID  int64
	TgChatID  int64
	Username  *string
	FirstName *string
	LastName  *string
}

type Service struct {
	ID          int64
	Name        string
	DurationMin int
	PriceMinor  int
}

type Master struct {
	ID       int64
	Name     string
	IsActive bool
}

type Appointment struct {
	ID        int64
	UserID    int64
	MasterID  int64
	ServiceID int64
	StartAt   time.Time // UTC
	EndAt     time.Time // UTC
	Status    string    // booked|confirmed|canceled|done
}

// Слоты: «момент начала» в локальном часовом поясе для удобства UI
type Slot struct {
	StartLocal time.Time
	EndLocal   time.Time
}

type SessionData struct {
	State   string
	Payload map[string]any // ваш booking payload (service/master/date/time и т.д.)
}

type Repo interface {
	// Пользователи
	UpsertUser(ctx context.Context, u User) (int64, error)
	GetUserByTG(ctx context.Context, tgUserID int64) (*User, error)

	// Каталоги
	ListActiveMasters(ctx context.Context) ([]Master, error)
	ListServicesByMaster(ctx context.Context, masterID int64) ([]Service, error)

	// Слоты (на основании working_hours, выходных и существующих записей)
	ListAvailableSlots(ctx context.Context, masterID, serviceID int64, day time.Time, loc *time.Location) ([]Slot, error)

	// Бронирование
	CreateAppointment(ctx context.Context, a Appointment) (int64, error)
	CancelAppointment(ctx context.Context, id int64) error
	ListUserAppointmentsUpcoming(ctx context.Context, userID int64, limit int) ([]Appointment, error)

	// FSM-сессия
	LoadSession(ctx context.Context, userID int64) (*SessionData, error)
	SaveSession(ctx context.Context, userID int64, s SessionData) error
}
