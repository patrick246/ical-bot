package refresh

import (
	"context"
	"net/http"
	"time"

	"github.com/emersion/go-ical"
)

type Calendar struct {
	client *http.Client
}

func NewCalendar(client *http.Client) *Calendar {
	return &Calendar{
		client: client,
	}
}

func (c *Calendar) Fetch(ctx context.Context, url string) (*ical.Calendar, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "github.com/patrick246/ical-bot")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	return ical.NewDecoder(resp.Body).Decode()
}

func (c *Calendar) AutoRefresh(ctx context.Context, url string, interval time.Duration) (<-chan *ical.Calendar, <-chan error) {
	calChan := make(chan *ical.Calendar, 1)
	errChan := make(chan error, 1)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			cal, err := c.Fetch(ctx, url)
			if err != nil {
				errChan <- err

				close(calChan)
				close(errChan)
			}

			calChan <- cal

			time.Sleep(interval)
		}
	}()

	return calChan, errChan
}
