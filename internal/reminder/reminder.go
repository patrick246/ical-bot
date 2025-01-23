package reminder

import (
	"context"
	"time"

	"github.com/emersion/go-ical"

	"github.com/patrick246/ical-bot/internal/refresh"
)

type Refresh interface {
	AutoRefresh(ctx context.Context, url string, interval time.Duration) (<-chan *ical.Calendar, <-chan error)
}

type Channel interface {
	Notify(ctx context.Context, event *ical.Event)
}

type Reminder struct {
	url      string
	channels []Channel

	last    *ical.Calendar
	calChan <-chan *ical.Calendar
	errchan <-chan error
}

func NewReminder(ctx context.Context, url string, interval time.Duration, calendar *refresh.Calendar, channels []Channel) *Reminder {
	calChan, errChan := calendar.AutoRefresh(ctx, url, interval)

	return &Reminder{
		url:      url,
		channels: channels,
		last:     nil,
		calChan:  calChan,
		errchan:  errChan,
	}
}

func (r *Reminder) CheckEvents() error {
	for {
		select {
		case newCal, ok := <-r.calChan:
			if !ok {
				return nil
			}

			r.last = newCal
		case err := <-r.errchan:
			return err
		default:
		}

	}
}

func extractEvents(cal *ical.Component) []ical.Event {
	events := make([]ical.Event, 0, len(cal.Children))

	for i := range cal.Children {
		if cal.Children[i].Name == ical.CompEvent {
			events = append(events, ical.Event{Component: cal.Children[i]})
		}

		childEvents := extractEvents(cal.Children[i])

		events = append(events, childEvents...)
	}

	return events
}
