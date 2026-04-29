package ui

import (
	"net/http"
	"testing"
	"testing/fstest"
)

func TestRenderUnauthenticated(t *testing.T) {
	rr := &mockResponseWriter{}
	RenderUnauthenticated(rr, nil)
	if rr.status != 200 {
		t.Errorf("expected 200, got %d", rr.status)
	}
}

func TestRenderAuthenticated(t *testing.T) {
	rr := &mockResponseWriter{}
	RenderAuthenticated(rr, nil)
	if rr.status != 200 {
		t.Errorf("expected 200, got %d", rr.status)
	}
}

func TestRenderAdmin(t *testing.T) {
	rr := &mockResponseWriter{}
	RenderAdmin(rr, map[string]any{"Users": nil})
	if rr.status != 200 {
		t.Errorf("expected 200, got %d", rr.status)
	}
}

func TestRenderUnauthenticated_WriteError(t *testing.T) {
	rr := &errorResponseWriter{}
	RenderUnauthenticated(rr, nil)
}

func TestRenderAuthenticated_WriteError(t *testing.T) {
	rr := &errorResponseWriter{}
	RenderAuthenticated(rr, nil)
}

func TestRenderAdmin_WriteError(t *testing.T) {
	rr := &errorResponseWriter{}
	RenderAdmin(rr, nil)
}

func TestMustInitTemplates_BadUnauthenticatedTemplate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for bad unauthenticated template")
		}
		// Restore valid templates
		mustInitTemplates(htmlFiles)
	}()

	// Provide a filesystem with an invalid (non-existent) unauthenticated template
	badFS := fstest.MapFS{}
	mustInitTemplates(badFS)
}

func TestMustInitTemplates_BadAuthenticatedTemplate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for bad authenticated template")
		}
		// Restore valid templates
		mustInitTemplates(htmlFiles)
	}()

	// Provide a filesystem with only unauthenticated template (missing authenticated)
	badFS := fstest.MapFS{
		"html/unauthenticated.html": &fstest.MapFile{Data: []byte(`Hello`)},
	}
	mustInitTemplates(badFS)
}

func TestMustInitTemplates_BadAdminTemplate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for bad admin template")
		}
		// Restore valid templates
		mustInitTemplates(htmlFiles)
	}()

	// Provide a filesystem missing admin template
	badFS := fstest.MapFS{
		"html/unauthenticated.html": &fstest.MapFile{Data: []byte(`Hello`)},
		"html/authenticated.html":   &fstest.MapFile{Data: []byte(`World`)},
	}
	mustInitTemplates(badFS)
}

// mockResponseWriter is a simple http.ResponseWriter for testing.
type mockResponseWriter struct {
	header http.Header
	status int
	body   []byte
}

func (m *mockResponseWriter) Header() http.Header {
	if m.header == nil {
		m.header = make(http.Header)
	}
	return m.header
}

func (m *mockResponseWriter) Write(b []byte) (int, error) {
	m.body = append(m.body, b...)
	return len(b), nil
}

func (m *mockResponseWriter) WriteHeader(code int) {
	m.status = code
}

// errorResponseWriter implements http.ResponseWriter but returns an error on Write.
type errorResponseWriter struct {
	header http.Header
}

func (e *errorResponseWriter) Header() http.Header {
	if e.header == nil {
		e.header = make(http.Header)
	}
	return e.header
}

func (e *errorResponseWriter) Write(b []byte) (int, error) {
	return 0, &writeError{}
}

func (e *errorResponseWriter) WriteHeader(code int) {}

type writeError struct{}

func (w *writeError) Error() string {
	return "simulated write error"
}
