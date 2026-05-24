package spinitron

import (
	"context"
	"errors"
	"testing"
	"time"

	appcatalog "air-cover/internal/app/catalog"
)

type fakePageClient struct {
	showPages    []ShowsPage
	personaPages []PersonasPage
	err          error
	showErr      error
	personaErr   error
	showCalls    int
	personaCalls int
}

func (f *fakePageClient) GetShowsPage(ctx context.Context, page int) (ShowsPage, error) {
	f.showCalls++
	if f.showErr != nil {
		return ShowsPage{}, f.showErr
	}
	if f.err != nil {
		return ShowsPage{}, f.err
	}
	if page <= 0 || page > len(f.showPages) {
		return ShowsPage{}, nil
	}
	return f.showPages[page-1], nil
}

func (f *fakePageClient) GetPersonasPage(ctx context.Context, page int) (PersonasPage, error) {
	f.personaCalls++
	if f.personaErr != nil {
		return PersonasPage{}, f.personaErr
	}
	if f.err != nil {
		return PersonasPage{}, f.err
	}
	if page <= 0 || page > len(f.personaPages) {
		return PersonasPage{}, nil
	}
	return f.personaPages[page-1], nil
}

func TestCatalog_ListShowsCachesPaginatedResults(t *testing.T) {
	next := 2
	source := &fakePageClient{
		showPages: []ShowsPage{
			{Items: []appcatalog.Show{{ID: "2", Title: "Second"}}, NextPage: &next},
			{Items: []appcatalog.Show{{ID: "1", Title: "First"}}},
		},
	}
	catalog := NewCatalog(source)

	shows, err := catalog.ListShows(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(shows) != 2 {
		t.Fatalf("expected 2 shows, got %d", len(shows))
	}
	shows[0].Title = "mutated"

	cached, err := catalog.ListShows(context.Background())
	if err != nil {
		t.Fatalf("expected cached call to succeed, got %v", err)
	}
	if source.showCalls != 2 {
		t.Fatalf("expected 2 upstream page calls, got %d", source.showCalls)
	}
	if cached[0].Title != "Second" {
		t.Fatalf("expected cached data to be cloned, got %q", cached[0].Title)
	}
}

func TestCatalog_ListPersonasCachesEmptyResults(t *testing.T) {
	source := &fakePageClient{personaPages: []PersonasPage{{}}}
	catalog := NewCatalog(source)

	personas, err := catalog.ListPersonas(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(personas) != 0 {
		t.Fatalf("expected no personas, got %d", len(personas))
	}

	_, err = catalog.ListPersonas(context.Background())
	if err != nil {
		t.Fatalf("expected cached empty result to succeed, got %v", err)
	}
	if source.personaCalls != 1 {
		t.Fatalf("expected empty result to be cached, got %d upstream calls", source.personaCalls)
	}
}

func TestCatalog_PrefetchLoadsBothCatalogs(t *testing.T) {
	source := &fakePageClient{
		showPages:    []ShowsPage{{Items: []appcatalog.Show{{ID: "1", Title: "Show"}}}},
		personaPages: []PersonasPage{{Items: []appcatalog.Persona{{ID: 1, Name: "DJ", Email: "dj@example.com"}}}},
	}
	catalog := NewCatalog(source)

	if err := catalog.Prefetch(context.Background()); err != nil {
		t.Fatalf("expected prefetch to succeed, got %v", err)
	}
	if source.showCalls != 1 || source.personaCalls != 1 {
		t.Fatalf("expected one fetch for each catalog, got shows=%d personas=%d", source.showCalls, source.personaCalls)
	}
}

func TestCatalog_PrefetchReturnsPersonaError(t *testing.T) {
	source := &fakePageClient{showPages: []ShowsPage{{}}, personaErr: errors.New("personas down")}
	catalog := NewCatalog(source)

	if err := catalog.Prefetch(context.Background()); err == nil {
		t.Fatal("expected persona prefetch error")
	}
}

func TestCatalog_PrefetchReturnsShowError(t *testing.T) {
	source := &fakePageClient{showErr: errors.New("shows down")}
	catalog := NewCatalog(source)

	if err := catalog.Prefetch(context.Background()); err == nil {
		t.Fatal("expected show prefetch error")
	}
}

func TestCatalog_ListShowsReturnsSourceError(t *testing.T) {
	source := &fakePageClient{err: errors.New("boom")}
	catalog := NewCatalog(source)

	if _, err := catalog.ListShows(context.Background()); err == nil {
		t.Fatal("expected source error")
	}
}

func TestCatalog_ListPersonasReturnsSourceError(t *testing.T) {
	source := &fakePageClient{err: errors.New("boom")}
	catalog := NewCatalog(source)

	if _, err := catalog.ListPersonas(context.Background()); err == nil {
		t.Fatal("expected source error")
	}
}

func TestCatalog_ListShowsValidatesSource(t *testing.T) {
	if _, err := (*Catalog)(nil).ListShows(context.Background()); err == nil {
		t.Fatal("expected nil catalog error")
	}

	catalog := NewCatalog(nil)
	if _, err := catalog.ListShows(context.Background()); err == nil {
		t.Fatal("expected nil source error")
	}
}

func TestCatalog_ListPersonasValidatesSource(t *testing.T) {
	catalog := NewCatalog(nil)
	if _, err := catalog.ListPersonas(context.Background()); err == nil {
		t.Fatal("expected nil source error")
	}
}

func TestCatalog_DefaultClockAndNilShowClone(t *testing.T) {
	catalog := NewCatalog(&fakePageClient{})
	catalog.nowFunc = nil
	if catalog.now().IsZero() {
		t.Fatal("expected default clock to return current time")
	}
	if cloneShows(nil) != nil {
		t.Fatal("expected nil show clone")
	}
}

func TestCatalog_FetchShowsStopsAtPageLimit(t *testing.T) {
	next := 1
	source := &fakePageClient{showPages: []ShowsPage{{NextPage: &next}}}
	catalog := NewCatalog(source)

	if _, err := catalog.ListShows(context.Background()); err == nil {
		t.Fatal("expected page limit error")
	}
}

func TestCatalog_FetchPersonasStopsAtPageLimit(t *testing.T) {
	next := 1
	source := &fakePageClient{personaPages: []PersonasPage{{NextPage: &next}}}
	catalog := NewCatalog(source)

	if _, err := catalog.ListPersonas(context.Background()); err == nil {
		t.Fatal("expected page limit error")
	}
}

func TestCatalog_WithRefreshTimeoutCanBeDisabled(t *testing.T) {
	source := &fakePageClient{showPages: []ShowsPage{{Items: []appcatalog.Show{{ID: "1", Title: "Show"}}}}}
	catalog := NewCatalog(source)
	catalog.refreshTimeout = 0

	if _, err := catalog.ListShows(context.Background()); err != nil {
		t.Fatalf("expected no error with disabled timeout, got %v", err)
	}
}

func TestCatalog_ExpiredShowsReturnStaleAndRefresh(t *testing.T) {
	now := time.Date(2026, time.May, 23, 12, 0, 0, 0, time.UTC)
	source := &fakePageClient{showPages: []ShowsPage{{Items: []appcatalog.Show{{ID: "1", Title: "Fresh"}}}}}
	catalog := NewCatalog(source)
	catalog.nowFunc = func() time.Time { return now }
	catalog.storeShows([]appcatalog.Show{{ID: "1", Title: "Stale"}})

	now = now.Add(defaultCatalogTTL + time.Second)
	shows, err := catalog.ListShows(context.Background())
	if err != nil {
		t.Fatalf("expected stale shows, got %v", err)
	}
	if shows[0].Title != "Stale" {
		t.Fatalf("expected stale show title, got %q", shows[0].Title)
	}

	deadline := time.After(time.Second)
	for {
		catalog.mu.Lock()
		refreshing := catalog.showsRefreshing
		title := catalog.shows[0].Title
		catalog.mu.Unlock()
		if !refreshing && title == "Fresh" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for background show refresh")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestCatalog_ExpiredPersonasReturnStaleAndRefresh(t *testing.T) {
	now := time.Date(2026, time.May, 23, 12, 0, 0, 0, time.UTC)
	source := &fakePageClient{personaPages: []PersonasPage{{Items: []appcatalog.Persona{{ID: 1, Name: "Fresh"}}}}}
	catalog := NewCatalog(source)
	catalog.nowFunc = func() time.Time { return now }
	catalog.storePersonas([]appcatalog.Persona{{ID: 1, Name: "Stale"}})

	now = now.Add(defaultCatalogTTL + time.Second)
	personas, err := catalog.ListPersonas(context.Background())
	if err != nil {
		t.Fatalf("expected stale personas, got %v", err)
	}
	if personas[0].Name != "Stale" {
		t.Fatalf("expected stale persona name, got %q", personas[0].Name)
	}

	deadline := time.After(time.Second)
	for {
		catalog.mu.Lock()
		refreshing := catalog.personasRefreshing
		name := catalog.personas[0].Name
		catalog.mu.Unlock()
		if !refreshing && name == "Fresh" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for background persona refresh")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
