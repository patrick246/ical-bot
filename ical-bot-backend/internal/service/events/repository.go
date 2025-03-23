package events

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/google/uuid"

	pb "github.com/patrick246/ical-bot/ical-bot-backend/internal/pkg/api/pb/ical-bot-backend/v1"
)

type timerange struct {
	from time.Time
	to   time.Time
}

func (t timerange) PostgresString() string {
	return fmt.Sprintf("['%s','%s')", t.from.Format(time.RFC3339Nano), t.to.Format(time.RFC3339Nano))
}

type timemultirange []timerange

func (t timemultirange) PostgresString() string {
	ranges := make([]string, 0, len(t))

	for _, r := range t {
		ranges = append(ranges, r.PostgresString())
	}

	return "{" + strings.Join(ranges, ", ") + "}"
}

type EventAlarm struct {
	ID        string
	EventID   string
	AlarmTime time.Time
	EventTime time.Time
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

type Import struct {
	calendarID string

	tx *sql.Tx
}

func (r *Repository) StartImport(ctx context.Context, calendarID string) (*Import, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx, `delete from calendar_events where calendar_id = $1`, calendarID)
	if err != nil {
		_ = tx.Rollback()

		return nil, err
	}

	return &Import{tx: tx, calendarID: calendarID}, nil
}

func (i *Import) Close(err error) error {
	if err != nil {
		return i.tx.Rollback()
	}

	_, err = i.tx.Exec(`UPDATE calendars SET last_sync_time = $1 WHERE id = $2`, time.Now(), i.calendarID)
	if err != nil {
		_ = i.tx.Rollback()
		return err
	}

	return i.tx.Commit()
}

func (i *Import) CreateEvent(ctx context.Context, calendar *pb.Calendar, event *ical.Event) error {
	eventID := uuid.New().String()

	data, err := encodeEvent(eventID, event)
	if err != nil {
		return err
	}

	alarms, err := calculateNextAlarms(calendar, eventID, event)
	if err != nil {
		return err
	}

	if len(alarms) == 0 {
		return nil
	}

	_, err = i.tx.ExecContext(ctx, `
		insert into calendar_events (id, calendar_id, data) values ($1, $2, $3);
	`, eventID, i.calendarID, data)

	if err != nil {
		return err
	}

	for _, alarm := range alarms {
		_, err := i.tx.ExecContext(ctx, `
			insert into calendar_event_alarms (event_id, alarm_time, event_time)
			values ($1, $2, $3)
		`, alarm.EventID, alarm.AlarmTime, alarm.EventTime)
		if err != nil {
			return err
		}
	}

	return nil
}

func encodeEvent(id string, event *ical.Event) ([]byte, error) {
	idProp := ical.NewProp(ical.PropProductID)
	idProp.SetText(id)

	versionProp := ical.NewProp(ical.PropVersion)
	versionProp.SetText("2.0")

	calendar := ical.NewCalendar()
	calendar.Props.Add(idProp)
	calendar.Props.Add(versionProp)
	calendar.Children = append(calendar.Children, event.Component)

	var buf bytes.Buffer
	err := ical.NewEncoder(&buf).Encode(calendar)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func calculateNextAlarms(calendar *pb.Calendar, eventID string, event *ical.Event) ([]EventAlarm, error) {
	eventStart, err := event.DateTimeStart(time.UTC)
	if err != nil {
		return nil, err
	}

	eventEnd, err := event.DateTimeEnd(time.UTC)
	if err != nil {
		return nil, err
	}

	var alarms []time.Duration

	switch calendar.DefaultReminderMode {
	case pb.DefaultReminderMode_DEFAULT_REMINDER_MODE_UNSET_ONLY:
		alarms = getAlarms(event)
		if len(alarms) == 0 {
			for _, defaultReminder := range calendar.DefaultReminders {
				alarms = append(alarms, defaultReminder.Before.AsDuration())
			}
		}
	case pb.DefaultReminderMode_DEFAULT_REMINDER_MODE_ADD:
		alarms = getAlarms(event)

		for _, defaultReminder := range calendar.DefaultReminders {
			alarms = append(alarms, defaultReminder.Before.AsDuration())
		}
	case pb.DefaultReminderMode_DEFAULT_REMINDER_MODE_REPLACE:
		for _, defaultReminder := range calendar.DefaultReminders {
			alarms = append(alarms, defaultReminder.Before.AsDuration())
		}
	}

	recurrenceSet, err := event.RecurrenceSet(time.UTC)
	if err != nil {
		return nil, err
	}

	if recurrenceSet == nil {
		if eventEnd.Before(time.Now()) {
			return nil, nil
		}

		nextAlarms := make([]EventAlarm, 0, len(alarms))

		for _, alarm := range alarms {
			nextAlarms = append(nextAlarms, EventAlarm{
				AlarmTime: eventStart.Add(-alarm),
				EventTime: eventStart,
				EventID:   eventID,
				ID:        uuid.New().String(),
			})
		}

		return nextAlarms, nil
	}

	nextAlarms := make([]EventAlarm, 0)
	it := recurrenceSet.Iterator()

	occurrenceCount := 0
	for {
		v, ok := it()
		if !ok {
			break
		}

		if v.Before(time.Now()) {
			continue
		}

		occurrenceCount++
		for _, alarm := range alarms {
			nextAlarms = append(nextAlarms, EventAlarm{
				ID:        uuid.New().String(),
				EventID:   eventID,
				AlarmTime: v.Add(-alarm),
				EventTime: v,
			})
		}

		if occurrenceCount >= 10 {
			break
		}
	}

	return nextAlarms, nil
}

func getAlarms(event *ical.Event) []time.Duration {
	alarms := make([]ical.Component, 0, len(event.Children))

	for _, component := range event.Children {
		if component.Name == ical.CompAlarm {
			alarms = append(alarms, *component)
		}
	}

	alarmsBefore := make([]time.Duration, 0, len(alarms))

	for _, alarm := range alarms {
		alarmProp := alarm.Props.Get(ical.PropTrigger)
		duration, err := alarmProp.Duration()
		if err != nil {
			continue
		}

		alarmsBefore = append(alarmsBefore, duration)
	}

	return alarmsBefore
}
