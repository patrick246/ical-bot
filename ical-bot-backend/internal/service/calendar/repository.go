package calendar

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/patrick246/ical-bot/ical-bot-backend/pkg/api/pb/ical-bot-backend/v1"
)

var ErrNotFound = errors.New("not found")

type Repository struct {
	db *sql.DB
}

func NewCalendarRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (c *Repository) CreateCalendar(ctx context.Context, calendar *pb.Calendar) (*pb.Calendar, error) {
	row := c.db.QueryRowContext(ctx, `
		insert into calendars (name, ical_url)
		values ($1, $2)
		returning id, name, ical_url, last_sync_time;
	`, calendar.Name, calendar.IcalUrl)

	return scanCalendar(row)
}

func (c *Repository) GetCalendar(
	ctx context.Context, id string,
) (*pb.Calendar, error) {
	return scanCalendar(c.db.QueryRowContext(ctx, `
		select id, name, ical_url, last_sync_time
		from calendars
		where id = $1
	`, id))
}

func (c *Repository) ListCalendars(
	ctx context.Context, pageSize int32, pageToken *pb.PageToken, filter *pb.ListCalendarsFilter,
) ([]*pb.Calendar, *pb.PageToken, error) {
	query := `
		select id, name, ical_url, last_sync_time
		from calendars
		where
			($2::uuid is null or id > $2) and
			($3::timestamptz is null or last_sync_time < $3)
		order by id
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

	var calendars []*pb.Calendar

	for rows.Next() {
		c, err := scanCalendar(rows)
		if err != nil {
			return nil, nil, err
		}

		calendars = append(calendars, c)
	}

	if rows.Err() != nil {
		return nil, nil, rows.Err()
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
			last_sync_time = coalesce($4, last_sync_time)
		where id = $1
		returning id, name, ical_url, last_sync_time
	`

	var (
		name         sql.Null[string]
		icalURL      sql.Null[string]
		lastSyncTime sql.Null[time.Time]
	)

	for _, p := range mask.GetPaths() {
		switch p {
		case "name":
			name = sql.Null[string]{V: calendar.Name, Valid: true}
		case "ical_url":
			icalURL = sql.Null[string]{V: calendar.IcalUrl, Valid: true}
		case "last_sync_time":
			lastSyncTime = sql.Null[time.Time]{V: calendar.LastSyncTime.AsTime(), Valid: true}
		}
	}

	return scanCalendar(c.db.QueryRowContext(
		ctx,
		query,
		name,
		icalURL,
		lastSyncTime,
		calendar.Id,
	))
}

func (c *Repository) DeleteCalendar(ctx context.Context, id string) error {
	_, err := c.db.ExecContext(ctx, `delete from calendars where id = $1`, id)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanCalendar(sc scanner) (*pb.Calendar, error) {
	var (
		calendar     = &pb.Calendar{}
		lastSyncTime sql.Null[time.Time]
	)

	err := sc.Scan(
		&calendar.Id,
		&calendar.Name,
		&calendar.IcalUrl,
		&lastSyncTime,
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

	return calendar, nil
}
