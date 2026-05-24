package spinitron

import "testing"

func TestConstructors(t *testing.T) {
	if NewClient("", "") == nil {
		t.Fatal("expected client")
	}
	if NewCatalog(nil) == nil {
		t.Fatal("expected catalog")
	}
}
