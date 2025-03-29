package events

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/emersion/go-ical"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/patrick246/ical-bot/ical-bot-backend/internal/pkg/api/pb/ical-bot-backend/v1"
)

var (
	ErrUnexpectedStatusCode = errors.New("unexpected HTTP status code")
	ErrIcalSizeExceeded     = errors.New("ical exceeded the maximum allowed size")
)

type CalendarRepository interface {
	ListCalendars(
		ctx context.Context, pageSize int32, pageToken *pb.PageToken, filter *pb.ListCalendarsFilter,
	) ([]*pb.Calendar, *pb.PageToken, error)
}

type EventRepository interface {
	StartImport(ctx context.Context, calendarID string) (*Import, error)
}

type IcalImport struct {
	eventRepo    EventRepository
	calendarRepo CalendarRepository
	httpClient   *http.Client
	logger       *slog.Logger
}

func NewIcalImport(
	eventRepo EventRepository, calendarRepo CalendarRepository, httpClient *http.Client, logger *slog.Logger,
) *IcalImport {
	return &IcalImport{
		eventRepo:    eventRepo,
		calendarRepo: calendarRepo,
		httpClient:   httpClient,
		logger:       logger,
	}
}

func (i *IcalImport) Run(ctx context.Context) error {
	nextPageToken := &pb.PageToken{}

	for {
		var (
			calendars []*pb.Calendar
			err       error
		)

		calendars, nextPageToken, err = i.calendarRepo.ListCalendars(ctx, 100, nextPageToken, &pb.ListCalendarsFilter{
			LastSyncTimeBefore: timestamppb.New(time.Now().Add(-5 * time.Minute)),
		})
		if err != nil {
			return err
		}

		eg := errgroup.Group{}
		eg.SetLimit(8)

		for _, cal := range calendars {
			eg.Go(func() error {
				err := i.importCalendar(ctx, cal)
				if err != nil {
					// Log, but don't return an error. We don't want to stop processing all calendars just because one is broken
					i.logger.Error("failed to import calendar",
						slog.String("calendar_id", cal.Id),
						slog.String("ical_url", cal.IcalUrl),
					)
				}

				return nil
			})
		}

		err = eg.Wait()
		if err != nil {
			return err
		}

		if nextPageToken == nil {
			break
		}
	}

	return nil
}

func (i *IcalImport) importCalendar(ctx context.Context, calendar *pb.Calendar) error {
	icalURL, err := url.ParseRequestURI(calendar.IcalUrl)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, icalURL.String(), http.NoBody)
	if err != nil {
		return err
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return fmt.Errorf("%w: %d", ErrUnexpectedStatusCode, resp.StatusCode)
	}

	lr := &io.LimitedReader{
		R: resp.Body,
		N: 10 * 1024 * 1024, // ToDo: define a sensible max size
	}

	decoder := ical.NewDecoder(lr)

	icalCalendar, err := decoder.Decode()
	if errors.Is(err, io.EOF) && lr.N == 0 {
		return ErrIcalSizeExceeded
	}
	if err != nil {
		return err
	}

	importOperation, err := i.eventRepo.StartImport(ctx, calendar.Id)
	if err != nil {
		return err
	}

	defer func() {
		_ = importOperation.Close(err)
	}()

	for _, ev := range icalCalendar.Events() {
		fmt.Printf("%#v\n", ev.Props.Get("SUMMARY"))

		err = importOperation.CreateEvent(ctx, calendar, &ev)
		if err != nil {
			return err
		}
	}

	return nil
}
