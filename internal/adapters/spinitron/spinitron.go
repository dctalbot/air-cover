package spinitron

import corespinitron "air-cover/internal/spinitron"

type PageClient = corespinitron.PageClient
type Catalog = corespinitron.Catalog
type Show = corespinitron.Show
type Persona = corespinitron.Persona
type ShowsPage = corespinitron.ShowsPage
type PersonasPage = corespinitron.PersonasPage

func NewClient(apiKey, baseURL string) PageClient {
	return corespinitron.NewClient(apiKey, baseURL)
}

func NewCatalog(source PageClient) *Catalog {
	return corespinitron.NewCatalog(source)
}
