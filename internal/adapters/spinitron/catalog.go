package spinitron

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	defaultCatalogTTL            = 15 * time.Minute
	defaultCatalogRefreshTimeout = 5 * time.Second
	maxCatalogPages              = 100
)

var errNoCatalogSource = errors.New("spinitron catalog source is not configured")

type PageClient interface {
	GetShowsPage(ctx context.Context, page int) (ShowsPage, error)
	GetPersonasPage(ctx context.Context, page int) (PersonasPage, error)
}

type Catalog struct {
	source         PageClient
	ttl            time.Duration
	refreshTimeout time.Duration
	nowFunc        func() time.Time

	mu                 sync.Mutex
	shows              []Show
	showsLoaded        bool
	showsExpiresAt     time.Time
	showsRefreshing    bool
	personas           []Persona
	personasLoaded     bool
	personasExpiresAt  time.Time
	personasRefreshing bool
}

func NewCatalog(source PageClient) *Catalog {
	return &Catalog{
		source:         source,
		ttl:            defaultCatalogTTL,
		refreshTimeout: defaultCatalogRefreshTimeout,
		nowFunc:        time.Now,
	}
}

func (c *Catalog) Prefetch(ctx context.Context) error {
	if _, err := c.ListShows(ctx); err != nil {
		return err
	}
	if _, err := c.ListPersonas(ctx); err != nil {
		return err
	}
	return nil
}

func (c *Catalog) ListShows(ctx context.Context) ([]Show, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	if shows, ok := c.cachedShows(); ok {
		return shows, nil
	}

	shows, err := c.fetchShows(c.withRefreshTimeout(ctx))
	if err != nil {
		return nil, err
	}
	c.storeShows(shows)
	return cloneShows(shows), nil
}

func (c *Catalog) ListPersonas(ctx context.Context) ([]Persona, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}

	if personas, ok := c.cachedPersonas(); ok {
		return personas, nil
	}

	personas, err := c.fetchPersonas(c.withRefreshTimeout(ctx))
	if err != nil {
		return nil, err
	}
	c.storePersonas(personas)
	return clonePersonas(personas), nil
}

func (c *Catalog) validate() error {
	if c == nil || c.source == nil {
		return errNoCatalogSource
	}
	return nil
}

func (c *Catalog) cachedShows() ([]Show, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.showsLoaded {
		return nil, false
	}

	shows := cloneShows(c.shows)
	if !c.now().Before(c.showsExpiresAt) && !c.showsRefreshing {
		c.showsRefreshing = true
		go c.refreshShows()
	}
	return shows, true
}

func (c *Catalog) cachedPersonas() ([]Persona, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.personasLoaded {
		return nil, false
	}

	personas := clonePersonas(c.personas)
	if !c.now().Before(c.personasExpiresAt) && !c.personasRefreshing {
		c.personasRefreshing = true
		go c.refreshPersonas()
	}
	return personas, true
}

func (c *Catalog) refreshShows() {
	shows, err := c.fetchShows(c.withRefreshTimeout(context.Background()))
	c.mu.Lock()
	defer c.mu.Unlock()
	defer func() { c.showsRefreshing = false }()
	if err == nil {
		c.shows = cloneShows(shows)
		c.showsLoaded = true
		c.showsExpiresAt = c.now().Add(c.ttl)
	}
}

func (c *Catalog) refreshPersonas() {
	personas, err := c.fetchPersonas(c.withRefreshTimeout(context.Background()))
	c.mu.Lock()
	defer c.mu.Unlock()
	defer func() { c.personasRefreshing = false }()
	if err == nil {
		c.personas = clonePersonas(personas)
		c.personasLoaded = true
		c.personasExpiresAt = c.now().Add(c.ttl)
	}
}

func (c *Catalog) storeShows(shows []Show) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shows = cloneShows(shows)
	c.showsLoaded = true
	c.showsExpiresAt = c.now().Add(c.ttl)
}

func (c *Catalog) storePersonas(personas []Persona) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.personas = clonePersonas(personas)
	c.personasLoaded = true
	c.personasExpiresAt = c.now().Add(c.ttl)
}

func (c *Catalog) fetchShows(ctx context.Context, cancel context.CancelFunc) ([]Show, error) {
	defer cancel()

	var shows []Show
	for page, fetched := 1, 0; page > 0; fetched++ {
		if fetched >= maxCatalogPages {
			return nil, fmt.Errorf("spinitron shows pagination exceeded %d pages", maxCatalogPages)
		}

		showsPage, err := c.source.GetShowsPage(ctx, page)
		if err != nil {
			return nil, err
		}
		shows = append(shows, showsPage.Items...)

		if showsPage.NextPage == nil {
			break
		}
		page = *showsPage.NextPage
	}
	return shows, nil
}

func (c *Catalog) fetchPersonas(ctx context.Context, cancel context.CancelFunc) ([]Persona, error) {
	defer cancel()

	var personas []Persona
	for page, fetched := 1, 0; page > 0; fetched++ {
		if fetched >= maxCatalogPages {
			return nil, fmt.Errorf("spinitron personas pagination exceeded %d pages", maxCatalogPages)
		}

		personasPage, err := c.source.GetPersonasPage(ctx, page)
		if err != nil {
			return nil, err
		}
		personas = append(personas, personasPage.Items...)

		if personasPage.NextPage == nil {
			break
		}
		page = *personasPage.NextPage
	}
	return personas, nil
}

func (c *Catalog) withRefreshTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.refreshTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.refreshTimeout)
}

func (c *Catalog) now() time.Time {
	if c.nowFunc != nil {
		return c.nowFunc()
	}
	return time.Now()
}

func cloneShows(shows []Show) []Show {
	if shows == nil {
		return nil
	}
	cloned := make([]Show, len(shows))
	copy(cloned, shows)
	return cloned
}

func clonePersonas(personas []Persona) []Persona {
	if personas == nil {
		return nil
	}
	cloned := make([]Persona, len(personas))
	copy(cloned, personas)
	return cloned
}
