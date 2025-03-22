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

func (i *Import) CreateEvent(ctx context.Context, event *ical.Event) error {
	eventID := uuid.New().String()

	data, err := encodeEvent(eventID, event)
	if err != nil {
		return err
	}

	occurrences, err := nextOccurrences(event)
	if err != nil {
		return err
	}

	if len(occurrences) == 0 {
		return nil
	}

	_, err = i.tx.ExecContext(ctx, `
		insert into calendar_events (id, calendar_id, data) VALUES ($1, $2, $3);
	`, eventID, i.calendarID, data)

	if err != nil {
		return err
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

func nextOccurrences(event *ical.Event) (timemultirange, error) {
	eventStart, err := event.DateTimeStart(time.UTC)
	if err != nil {
		return nil, err
	}

	eventEnd, err := event.DateTimeEnd(time.UTC)
	if err != nil {
		return nil, err
	}

	eventDuration := eventEnd.Sub(eventStart)

	recurrenceSet, err := event.RecurrenceSet(time.UTC)
	if err != nil {
		return nil, err
	}

	if recurrenceSet == nil {
		if eventEnd.Before(time.Now()) {
			return nil, nil
		}

		return timemultirange{
			{eventStart, eventEnd},
		}, nil
	}

	occurrences := make(timemultirange, 0, 10)
	it := recurrenceSet.Iterator()

	for {
		v, ok := it()
		if !ok {
			break
		}

		if v.Before(time.Now()) {
			continue
		}

		occurrences = append(occurrences, timerange{
			from: v,
			to:   v.Add(eventDuration),
		})

		if len(occurrences) >= 10 {
			break
		}
	}

	return occurrences, nil
}

func getAlarms(event *ical.Event) []ical.Component {
	alarms := make([]ical.Component, 0, len(event.Children))

	for _, component := range event.Children {
		if component.Name == ical.CompAlarm {
			alarms = append(alarms, *component)
		}
	}

	return alarms
}
