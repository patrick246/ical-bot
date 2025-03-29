package calendar

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	pbStatus "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/patrick246/ical-bot/ical-bot-backend/internal/pkg/api/pb/ical-bot-backend/v1"
)

var ErrNotFound = errors.New("not found")

type Repository struct {
	db *sql.DB
}

type DefaultReminderRecord struct {
	ID         string          `json:"id"`
	CalendarID string          `json:"calendar_id"`
	Before     pgtype.Interval `json:"before"`
}

func NewCalendarRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (c *Repository) CreateCalendar(ctx context.Context, calendar *pb.Calendar) (*pb.Calendar, error) {
	calendar.Id = uuid.New().String()

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}

	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		insert into calendars (id, name, ical_url, default_reminder_mode)
		values ($1, $2, $3, $4);
	`, calendar.Id, calendar.Name, calendar.IcalUrl, calendar.DefaultReminderMode.String())
	if err != nil {
		return nil, err
	}

	defaultRemindersQuery := sq.Insert("calendar_default_reminders").Columns("id", "calendar_id", "before")

	for _, defaultReminders := range calendar.DefaultReminders {
		defaultRemindersQuery.Values(defaultReminders.Id, calendar.Id, defaultReminders.Before.AsDuration())
	}

	query, val, err := defaultRemindersQuery.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx, query, val...)
	if err != nil {
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return c.GetCalendar(ctx, calendar.Id)
}

func (c *Repository) GetCalendar(
	ctx context.Context, id string,
) (*pb.Calendar, error) {
	calendar, err := scanCalendar(c.db.QueryRowContext(ctx, `
		select c.id, name, ical_url, last_sync_time, last_sync_hash, sync_error_pb, default_reminder_mode
		from calendars c
		where c.id = $1
	`, id))
	if err != nil {
		return nil, err
	}

	reminders, err := c.getDefaultReminders(ctx, []string{calendar.Id})
	if err != nil {
		return nil, err
	}

	calendar.DefaultReminders = reminders[calendar.Id]

	return calendar, nil
}

func (c *Repository) ListCalendars(
	ctx context.Context, pageSize int32, pageToken *pb.PageToken, filter *pb.ListCalendarsFilter,
) ([]*pb.Calendar, *pb.PageToken, error) {
	query := `
		select c.id, name, ical_url, last_sync_time, last_sync_hash, sync_error_pb, default_reminder_mode
		from calendars c
		where
			($2::uuid is null or c.id > $2) and
			($3::timestamptz is null or last_sync_time < $3)
		order by c.id
		limit $1
	`

	var lastID *string
	if pageToken.LastId != "" {
		lastID = &pageToken.LastId
	}

	rows, err := c.db.QueryContext(ctx, query, pageSize, lastID, filter.LastSyncTimeBefore.AsTime())
	if err != nil {
		return nil, nil, err
	}

	var (
		calendars   []*pb.Calendar
		calendarIDs []string
	)

	for rows.Next() {
		c, err := scanCalendar(rows)
		if err != nil {
			return nil, nil, err
		}

		calendars = append(calendars, c)
		calendarIDs = append(calendarIDs, c.Id)
	}

	if rows.Err() != nil {
		return nil, nil, rows.Err()
	}

	reminders, err := c.getDefaultReminders(ctx, calendarIDs)
	if err != nil {
		return nil, nil, err
	}

	for i := range calendars {
		calendars[i].DefaultReminders = reminders[calendars[i].Id]
	}

	var nextPageToken *pb.PageToken

	if int32(len(calendars)) == pageSize {
		nextPageToken = &pb.PageToken{
			LastId: calendars[len(calendars)-1].Id,
		}
	}

	return calendars, nextPageToken, nil
}

func (c *Repository) UpdateCalendar(
	ctx context.Context, calendar *pb.Calendar, mask *fieldmaskpb.FieldMask,
) (*pb.Calendar, error) {
	query := `
		update calendars set
			name = coalesce($2, name),
			ical_url = coalesce($3, ical_url),
			last_sync_time = coalesce($4, last_sync_time),
			last_sync_hash = coalesce($5, last_sync_hash),
			sync_error_pb = coalesce($6, sync_error_pb)
		where id = $1
		returning id, name, ical_url, last_sync_time, last_sync_hash, sync_error_pb, default_reminder_mode
	`

	var (
		name         sql.Null[string]
		icalURL      sql.Null[string]
		lastSyncTime sql.Null[time.Time]
		lastSyncHash sql.Null[[]byte]
		syncError    sql.Null[[]byte]
	)

	for _, p := range mask.GetPaths() {
		switch p {
		case "name":
			name = sql.Null[string]{V: calendar.Name, Valid: true}
		case "ical_url":
			icalURL = sql.Null[string]{V: calendar.IcalUrl, Valid: true}
		case "last_sync_time":
			lastSyncTime = sql.Null[time.Time]{V: calendar.LastSyncTime.AsTime(), Valid: true}
		case "last_sync_hash":
			lastSyncHash = sql.Null[[]byte]{V: calendar.LastSyncHash, Valid: true}
		case "last_sync_error":
			syncErrorBytes, err := proto.Marshal(calendar.LastSyncError)
			if err != nil {
				return nil, err
			}

			syncError = sql.Null[[]byte]{V: syncErrorBytes, Valid: true}
		}
	}

	return scanCalendar(c.db.QueryRowContext(
		ctx,
		query,
		calendar.Id,
		name,
		icalURL,
		lastSyncTime,
		lastSyncHash,
		syncError,
	))
}

func (c *Repository) DeleteCalendar(ctx context.Context, id string) error {
	_, err := c.db.ExecContext(ctx, `delete from calendars where id = $1`, id)
	return err
}

func (c *Repository) getDefaultReminders(
	ctx context.Context, calendarIDs []string,
) (map[string][]*pb.DefaultReminder, error) {
	rows, err := c.db.QueryContext(ctx, `
		select id, calendar_id, before
		from calendar_default_reminders
		where calendar_id = any($1::uuid[])
	`, calendarIDs)
	if err != nil {
		return nil, err
	}

	defaultReminders := make(map[string][]*pb.DefaultReminder, len(calendarIDs))

	for rows.Next() {
		defaultReminder, calendarID, err := scanDefaultReminder(rows)
		if err != nil {
			return nil, err
		}

		defaultReminders[calendarID] = append(defaultReminders[calendarID], defaultReminder)
	}

	return defaultReminders, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanCalendar(sc scanner) (*pb.Calendar, error) {
	var (
		calendar            = &pb.Calendar{}
		lastSyncTime        sql.Null[time.Time]
		lastSyncError       sql.Null[[]byte]
		defaultReminderMode sql.Null[string]
	)

	err := sc.Scan(
		&calendar.Id,
		&calendar.Name,
		&calendar.IcalUrl,
		&lastSyncTime,
		&calendar.LastSyncHash,
		&lastSyncError,
		&defaultReminderMode,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	if lastSyncTime.Valid {
		calendar.LastSyncTime = timestamppb.New(lastSyncTime.V)
	}

	if lastSyncError.Valid {
		var status pbStatus.Status
		err := proto.Unmarshal(lastSyncError.V, &status)
		if err != nil {
			return nil, err
		}

		calendar.LastSyncError = &status
	}

	if defaultReminderMode.Valid {
		calendar.DefaultReminderMode = pb.DefaultReminderMode(pb.DefaultReminderMode_value[defaultReminderMode.V])
	}

	return calendar, nil
}

func scanDefaultReminder(sc scanner) (*pb.DefaultReminder, string, error) {
	var (
		defaultReminder = &pb.DefaultReminder{}
		before          pgtype.Interval
		calendarID      string
	)

	err := sc.Scan(
		&defaultReminder.Id,
		&calendarID,
		&before,
	)
	if err != nil {
		return nil, "", err
	}

	if before.Valid {
		defaultReminder.Before = durationpb.New(time.Duration(before.Microseconds)*time.Microsecond + 24*time.Hour*time.Duration(before.Days) + 30*24*time.Hour*time.Duration(before.Months))
	}

	return defaultReminder, calendarID, nil
}

func pgIntervalToDuration(in pgtype.Interval) time.Duration {
	return time.Duration(in.Microseconds) + time.Duration(in.Days)*24*time.Hour + time.Duration(in.Months)*30*24*time.Hour
}
