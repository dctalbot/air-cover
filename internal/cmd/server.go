package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"air-cover/internal/api"
	"air-cover/internal/config"
	"air-cover/internal/db"
	"air-cover/internal/email"
	"air-cover/internal/spinitron"
)

var (
	osExit         = os.Exit
	listenAndServe = func(server *http.Server) error {
		return server.ListenAndServe()
	}
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the web server",
	Long:  `Start the Air Cover web server.`,
	Run: func(cmd *cobra.Command, args []string) {
		slog.Info("Starting Air Cover server...")

		cfg, err := config.Load(cmd)
		if err != nil {
			slog.Error("Failed to load configuration", "error", err)
			osExit(1)
		}

		database, err := db.InitDB(cfg.DBURI)
		if err != nil {
			slog.Error("Failed to initialize database", "error", err)
			osExit(1)
		}
		repo := db.NewRepository(database)

		if cfg.MasterEmail != "" {
			_, err := repo.GetUserByEmail(context.Background(), cfg.MasterEmail)
			if errors.Is(err, db.ErrNotFound) {
				slog.Info("Creating master admin user", "email", cfg.MasterEmail)
				_, err = repo.CreateUser(context.Background(), cfg.MasterEmail)
				if err != nil {
					slog.Error("Failed to create master user", "error", err)
					osExit(1)
				}
			} else if err != nil {
				slog.Error("Failed to check master user", "error", err)
				osExit(1)
			}
		}

		sender := email.NewSender(cfg.SendGridAPIKey, cfg.ENV)
		authHandler := api.NewAuthHandler(repo, sender)
		spinitronClient := spinitron.NewClient("", cfg.SpinitronAPIURL)

		mux := http.NewServeMux()
		mux.HandleFunc("/", indexHandler(repo))
		mux.Handle("/app", authHandler.AuthMiddleware(appHandler()))
		mux.Handle("/shows", authHandler.AuthMiddleware(showsHandler(spinitronClient)))
		mux.HandleFunc("/health", healthHandler)
		mux.HandleFunc("/auth/login", authHandler.HandleLogin)
		mux.HandleFunc("/auth/verify", authHandler.HandleVerify)

		portStr := strconv.Itoa(cfg.Port)
		slog.Info("Listening on port", "port", portStr)

		server := &http.Server{
			Addr:              ":" + portStr,
			Handler:           mux,
			ReadHeaderTimeout: 3 * time.Second,
		}

		if err := listenAndServe(server); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server failed to start", "error", err)
			osExit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	_ = viper.BindPFlag("port", serverCmd.Flags().Lookup("port"))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("OK"))
	if err != nil {
		slog.Error("Failed to write response", "error", err)
	}
}

const unauthenticatedIndexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Air Cover Login</title>
</head>
<body>
    <h1>Air Cover</h1>
    <p>Enter your email to receive a magic login link.</p>
    <form id="magic-link-form">
        <label for="email">Email</label>
        <input type="email" id="email" name="email" required />
        <button type="submit">Send magic link</button>
    </form>
    <p id="message" role="status"></p>
    <script>
        const form = document.getElementById('magic-link-form');
        const message = document.getElementById('message');

        form.addEventListener('submit', async (event) => {
            event.preventDefault();
            const email = document.getElementById('email').value.trim();

            if (!email) {
                message.textContent = 'Please provide an email address.';
                return;
            }

            message.textContent = 'Sending magic link...';

            try {
                const response = await fetch('/auth/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ email: email })
                });

                if (!response.ok) {
                    throw new Error('Failed to send link');
                }

                const payload = await response.json();
                message.textContent = payload.message || 'Check your inbox for your link.';
            } catch (error) {
                message.textContent = 'Unable to send link right now. Please try again.';
            }
        });
    </script>
</body>
</html>`

const authenticatedIndexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Air Cover</title>
</head>
<body>
    <h1>Welcome to Air Cover</h1>
    <p>Select a show to continue.</p>
    <label for="show-select">Shows</label>
    <select id="show-select" name="show">
        <option value="">Loading shows...</option>
    </select>
    <script>
        const showSelect = document.getElementById('show-select');

        const fetchAllShows = async () => {
            const allShows = [];
            let page = 1;

            while (page) {
                const response = await fetch('/shows?page=' + page);
                if (!response.ok) {
                    throw new Error('Failed to load shows');
                }

                const payload = await response.json();
                const items = Array.isArray(payload.items) ? payload.items : [];
                allShows.push(...items);

                page = payload.next_page || null;
            }

            return allShows;
        };

        const renderShows = (shows) => {
            showSelect.innerHTML = '';
            if (shows.length === 0) {
                showSelect.innerHTML = '<option value="">No shows available</option>';
                return;
            }

            showSelect.innerHTML = '<option value="">Select a show</option>';
            for (const show of shows) {
                const option = document.createElement('option');
                option.value = show.id;
                option.textContent = show.title || show.id;
                showSelect.appendChild(option);
            }
        };

        const loadShows = async () => {
            try {
                const shows = await fetchAllShows();
                renderShows(shows);
            } catch (error) {
                showSelect.innerHTML = '<option value="">Unable to load shows</option>';
            }
        };

        loadShows();
    </script>
</body>
</html>`

func indexHandler(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		if cookie, err := r.Cookie("session_id"); err == nil && cookie.Value != "" {
			if _, err := repo.GetSessionByToken(r.Context(), cookie.Value); err == nil {
				http.Redirect(w, r, "/app", http.StatusFound)
				return
			}

			// Clear stale session cookie so users can request a fresh login link.
			http.SetCookie(w, &http.Cookie{
				Name:     "session_id",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}

		writeHTML(w, unauthenticatedIndexHTML)
	}
}

func appHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app" {
			http.NotFound(w, r)
			return
		}

		writeHTML(w, authenticatedIndexHTML)
	}
}

type showsService interface {
	GetShowsPage(ctx context.Context, page int) (spinitron.ShowsPage, error)
}

func showsHandler(client showsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		page := 1
		pageRaw := r.URL.Query().Get("page")
		if pageRaw != "" {
			parsedPage, err := strconv.Atoi(pageRaw)
			if err != nil || parsedPage < 1 {
				http.Error(w, "Invalid page parameter", http.StatusBadRequest)
				return
			}
			page = parsedPage
		}

		showsPage, err := client.GetShowsPage(r.Context(), page)
		if err != nil {
			slog.Error("Failed to load shows from spinitron", "error", err)
			http.Error(w, "Unable to load shows", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(showsPage); err != nil {
			slog.Error("Failed to encode shows response", "error", err)
		}
	}
}

func writeHTML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(body))
	if err != nil {
		slog.Error("Failed to write response", "error", err)
	}
}
