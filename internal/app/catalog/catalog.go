package catalog

type Show struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type Persona struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}
