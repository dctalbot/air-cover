package httpadapter

import (
	"net/http"

	"air-cover/internal/domain"
)

func currentUser(r *http.Request) domain.CurrentUser {
	viewer, _ := userFromContext(r)
	return viewer
}

func requireCurrentUser(w http.ResponseWriter, r *http.Request) (domain.CurrentUser, bool) {
	viewer, ok := userFromContext(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return domain.CurrentUser{}, false
	}
	return viewer, true
}

func userFromContext(r *http.Request) (domain.CurrentUser, bool) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		return domain.CurrentUser{}, false
	}
	email, _ := r.Context().Value(UserEmailKey).(string)
	role, _ := r.Context().Value(UserRoleKey).(domain.Role)
	return domain.CurrentUser{
		ID:    userID,
		Email: email,
		Role:  role,
	}, true
}
