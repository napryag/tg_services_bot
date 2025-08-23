package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/napryag/tg_services_bot/pkg/repository/model"
)

type PGRepo struct{ pool *pgxpool.Pool }

func NewRepo(ctx context.Context, dsn string) (*PGRepo, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &PGRepo{pool: pool}, nil
}

func (r *PGRepo) UpsertUser(ctx context.Context, u model.User) (int64, error) {
	q := `
		INSERT INTO app_user (tg_user_id, tg_chat_id, username, first_name, last_name)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (tg_user_id) DO UPDATE
		   SET tg_chat_id = EXCLUDED.tg_chat_id,
		       username   = COALESCE(EXCLUDED.username, app_user.username),
		       first_name = COALESCE(EXCLUDED.first_name, app_user.first_name),
		       last_name  = COALESCE(EXCLUDED.last_name, app_user.last_name),
		       updated_at = now()
		RETURNING id;
	`
	var id int64
	err := r.pool.QueryRow(ctx, q, u.TgUserID, u.TgChatID, u.Username, u.FirstName, u.LastName).Scan(&id)
	return id, err
}

func (r *PGRepo) ListActiveMasters(ctx context.Context) ([]model.Master, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,name,is_active FROM master WHERE is_active ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Master
	for rows.Next() {
		var m model.Master
		if err := rows.Scan(&m.ID, &m.Name, &m.IsActive); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *PGRepo) ListServicesByMaster(ctx context.Context, masterID int64) ([]model.Service, error) {
	const q = `
		SELECT s.id, s.name, s.duration_min, s.price_minor
		FROM master_service ms
		JOIN service s ON s.id = ms.service_id
		WHERE ms.master_id = $1
		ORDER BY s.name;
	`
	rows, err := r.pool.Query(ctx, q, masterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Service
	for rows.Next() {
		var s model.Service
		if err := rows.Scan(&s.ID, &s.Name, &s.DurationMin, &s.PriceMinor); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PGRepo) ListAvailableSlots(ctx context.Context, masterID, serviceID int64, day time.Time, loc *time.Location) ([]model.Slot, error) {
	day = day.In(loc)
	weekday := int(day.Weekday()) // 0=Sunday

	// 1) Услуга
	var durationMin int
	if err := r.pool.QueryRow(ctx, `SELECT duration_min FROM service WHERE id=$1`, serviceID).Scan(&durationMin); err != nil {
		return nil, err
	}
	step := time.Duration(durationMin) * time.Minute

	// 2) Рабочие часы и выходные
	var tStart, tEnd time.Time
	{
		var st, en time.Time
		// берём локальную дату+время (time type -> отн. 0001-01-01), комбинируем с датой
		err := r.pool.QueryRow(ctx, `SELECT time_start::time, time_end::time FROM working_hours WHERE master_id=$1 AND dow=$2`, masterID, weekday).Scan(&st, &en)
		if err != nil {
			// нет расписания — нет слотов
			return []model.Slot{}, nil
		}
		year, month, dayN := day.Date()
		tStart = time.Date(year, month, dayN, st.Hour(), st.Minute(), st.Second(), 0, loc)
		tEnd = time.Date(year, month, dayN, en.Hour(), en.Minute(), en.Second(), 0, loc)

		var dummy int
		err = r.pool.QueryRow(ctx, `SELECT 1 FROM day_off WHERE master_id=$1 AND day=$2::date`, masterID, day).Scan(&dummy)
		if err == nil {
			return []model.Slot{}, nil // выходной день
		}
	}

	// 3) Забронированные интервалы (UTC → локаль)
	const qBusy = `
		SELECT start_at, end_at
		FROM appointment
		WHERE master_id=$1
		  AND status IN ('booked','confirmed')
		  AND start_at::date <= ($2::date + INTERVAL '1 day')
		  AND end_at::date >= $2::date;
	`
	rows, err := r.pool.Query(ctx, qBusy, masterID, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type iv struct{ a, b time.Time }
	var busy []iv
	for rows.Next() {
		var aUTC, bUTC time.Time
		if err := rows.Scan(&aUTC, &bUTC); err != nil {
			return nil, err
		}
		busy = append(busy, iv{a: aUTC.In(loc), b: bUTC.In(loc)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	overlaps := func(a1, a2, b1, b2 time.Time) bool { return a1.Before(b2) && b1.Before(a2) }

	var slots []model.Slot
	for t := tStart; t.Add(step).Equal(tEnd) || t.Add(step).Before(tEnd); t = t.Add(step) {
		s := t
		e := t.Add(step)
		// проверка пересечения
		conflict := false
		for _, iv := range busy {
			if overlaps(s, e, iv.a, iv.b) {
				conflict = true
				break
			}
		}
		if !conflict {
			slots = append(slots, model.Slot{StartLocal: s, EndLocal: e})
		}
	}
	return slots, nil
}

func (r *PGRepo) CreateAppointment(ctx context.Context, a model.Appointment) (int64, error) {
	// Вставляем. Если пересечение — сработает EXCLUDE no_overlap -> ошибка 23P01
	const q = `
		INSERT INTO appointment (user_id, master_id, service_id, start_at, end_at, status)
		VALUES ($1,$2,$3,$4,$5,'booked')
		RETURNING id;
	`
	var id int64
	err := r.pool.QueryRow(ctx, q, a.UserID, a.MasterID, a.ServiceID, a.StartAt, a.EndAt).Scan(&id)
	if err != nil {
		// код ошибки уникального/исключающего ограничения
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && (pgerr.Code == "23P01" || pgerr.Code == "23505") {
			return 0, fmt.Errorf("slot_taken")
		}
		return 0, err
	}
	return id, nil
}

func (r *PGRepo) CancelAppointment(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE appointment SET status='canceled' WHERE id=$1`, id)
	return err
}

func (r *PGRepo) ListUserAppointmentsUpcoming(ctx context.Context, userID int64, limit int) ([]model.Appointment, error) {
	const q = `
		SELECT id, user_id, master_id, service_id, start_at, end_at, status
		FROM appointment
		WHERE user_id=$1 AND status IN ('booked','confirmed') AND start_at >= now()
		ORDER BY start_at
		LIMIT $2;
	`
	rows, err := r.pool.Query(ctx, q, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Appointment
	for rows.Next() {
		var a model.Appointment
		if err := rows.Scan(&a.ID, &a.UserID, &a.MasterID, &a.ServiceID, &a.StartAt, &a.EndAt, &a.Status); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *PGRepo) LoadSession(ctx context.Context, userID int64) (*model.SessionData, error) {
	var s model.SessionData
	var payload []byte
	err := r.pool.QueryRow(ctx, `SELECT state, payload FROM user_session WHERE user_id=$1`, userID).Scan(&s.State, &payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &model.SessionData{State: "main", Payload: map[string]any{}}, nil
		}
		return nil, err
	}
	_ = json.Unmarshal(payload, &s.Payload)
	return &s, nil
}

func (r *PGRepo) SaveSession(ctx context.Context, userID int64, s model.SessionData) error {
	pb, _ := json.Marshal(s.Payload)
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_session (user_id, state, payload, updated_at)
		VALUES ($1,$2,$3,now())
		ON CONFLICT (user_id) DO UPDATE
		   SET state=EXCLUDED.state, payload=EXCLUDED.payload, updated_at=now()
	`, userID, s.State, pb)
	return err
}
