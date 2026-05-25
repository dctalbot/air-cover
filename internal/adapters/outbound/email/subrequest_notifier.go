package email

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	subrequestsapp "air-cover/internal/app/subrequests"
)

const defaultAsyncNotifierBuffer = 32

type SubRequestNotifier struct {
	Users   subrequestsapp.ActiveUserLister
	Sender  Sender
	BaseURL string
}

func (n *SubRequestNotifier) SubRequestCreated(ctx context.Context, event subrequestsapp.SubRequestCreatedEvent) error {
	if n.Users == nil {
		return fmt.Errorf("active user lister is not configured")
	}
	if n.Sender == nil {
		return fmt.Errorf("email sender is not configured")
	}
	users, err := n.Users.ListActiveUsers(ctx)
	if err != nil {
		return fmt.Errorf("list active users: %w", err)
	}
	recipients := make([]string, 0, len(users))
	for _, user := range users {
		recipients = append(recipients, user.Email)
	}
	if len(recipients) == 0 {
		return nil
	}

	message := SubRequestCreatedMessage{
		ShowTitle:      event.ShowTitle,
		RequesterEmail: event.RequesterEmail,
		StartTime:      event.Request.StartTime,
		EndTime:        event.Request.EndTime,
		Notes:          event.Request.Notes,
		DetailURL:      n.detailURL(event.DetailPath),
	}
	if err := n.Sender.SendSubRequestCreated(recipients, message); err != nil {
		slog.Error("Failed to send sub request notification email", "bcc_count", len(recipients), "sub_request_id", event.Request.ID, "error", err)
	}
	return nil
}

func (n *SubRequestNotifier) SubRequestTaken(ctx context.Context, event subrequestsapp.SubRequestTakenEvent) error {
	if n.Sender == nil {
		return fmt.Errorf("email sender is not configured")
	}
	if event.RequesterEmail == "" {
		return nil
	}

	message := SubRequestTakenMessage{
		ShowTitle:  event.ShowTitle,
		TakerEmail: event.TakerEmail,
		StartTime:  event.Request.StartTime,
		EndTime:    event.Request.EndTime,
		DetailURL:  n.detailURL(event.DetailPath),
	}
	ccEmails := []string(nil)
	if event.TakerEmail != "" {
		ccEmails = []string{event.TakerEmail}
	}
	if err := n.Sender.SendSubRequestTaken(event.RequesterEmail, ccEmails, message); err != nil {
		slog.Error("Failed to send sub request taken confirmation email", "to", event.RequesterEmail, "cc_count", len(ccEmails), "sub_request_id", event.Request.ID, "error", err)
	}
	return nil
}

func (n *SubRequestNotifier) detailURL(path string) string {
	base := strings.TrimRight(n.BaseURL, "/")
	if base == "" {
		return path
	}
	relative := "/" + strings.TrimLeft(path, "/")
	return base + relative
}

type AsyncNotifier struct {
	next subrequestsapp.Notifier
	jobs chan asyncNotification
	done chan struct{}
	once sync.Once
	wg   sync.WaitGroup
}

type asyncNotification struct {
	ctx          context.Context
	createdEvent subrequestsapp.SubRequestCreatedEvent
	takenEvent   subrequestsapp.SubRequestTakenEvent
	kind         asyncNotificationKind
}

type asyncNotificationKind string

const (
	asyncNotificationCreated asyncNotificationKind = "created"
	asyncNotificationTaken   asyncNotificationKind = "taken"
)

func NewAsyncNotifier(next subrequestsapp.Notifier, buffer int) *AsyncNotifier {
	if buffer <= 0 {
		buffer = defaultAsyncNotifierBuffer
	}
	return &AsyncNotifier{
		next: next,
		jobs: make(chan asyncNotification, buffer),
		done: make(chan struct{}),
	}
}

func (n *AsyncNotifier) Start() {
	if n == nil || n.next == nil {
		return
	}
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		for job := range n.jobs {
			if err := n.process(job); err != nil {
				slog.Error("Failed to process sub request notification", "sub_request_id", job.subRequestID(), "error", err)
			}
		}
		close(n.done)
	}()
}

func (n *AsyncNotifier) process(job asyncNotification) error {
	switch job.kind {
	case asyncNotificationCreated:
		return n.next.SubRequestCreated(job.ctx, job.createdEvent)
	case asyncNotificationTaken:
		return n.next.SubRequestTaken(job.ctx, job.takenEvent)
	default:
		return nil
	}
}

func (j asyncNotification) subRequestID() int {
	switch j.kind {
	case asyncNotificationCreated:
		if j.createdEvent.Request != nil {
			return j.createdEvent.Request.ID
		}
	case asyncNotificationTaken:
		if j.takenEvent.Request != nil {
			return j.takenEvent.Request.ID
		}
	}
	return 0
}

func (n *AsyncNotifier) Stop() {
	if n == nil {
		return
	}
	n.once.Do(func() {
		close(n.jobs)
		n.wg.Wait()
	})
}

func (n *AsyncNotifier) Done() <-chan struct{} {
	if n == nil {
		done := make(chan struct{})
		close(done)
		return done
	}
	return n.done
}

func (n *AsyncNotifier) SubRequestCreated(ctx context.Context, event subrequestsapp.SubRequestCreatedEvent) error {
	if n == nil || n.next == nil {
		return nil
	}
	select {
	case n.jobs <- asyncNotification{ctx: contextWithoutCancel(ctx), createdEvent: event, kind: asyncNotificationCreated}:
		return nil
	default:
		return fmt.Errorf("sub request notification queue is full")
	}
}

func (n *AsyncNotifier) SubRequestTaken(ctx context.Context, event subrequestsapp.SubRequestTakenEvent) error {
	if n == nil || n.next == nil {
		return nil
	}
	select {
	case n.jobs <- asyncNotification{ctx: contextWithoutCancel(ctx), takenEvent: event, kind: asyncNotificationTaken}:
		return nil
	default:
		return fmt.Errorf("sub request notification queue is full")
	}
}

func contextWithoutCancel(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}
