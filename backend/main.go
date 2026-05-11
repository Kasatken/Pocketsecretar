package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "github.com/lib/pq"

	"pocketsecret/config"
	"pocketsecret/handlers"
	"pocketsecret/middleware"
	"pocketsecret/migrations"
	"pocketsecret/repository"
	"pocketsecret/security"
)

func main() {
	cfg := config.Load()
	db := initDB(cfg.DatabaseURL)
	defer db.Close()

	if err := migrations.Run(context.Background(), db); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(cfg.UploadDir, "avatars"), 0755); err != nil {
		log.Fatalf("upload dir: %v", err)
	}

	users := repository.NewUserRepository(db)
	googleLinks := repository.NewGoogleCalendarRepository(db)
	backlogIdeas := repository.NewBacklogIdeaRepository(db)
	smartTaskQuota := repository.NewSmartTaskRepository(db)
	dailyAdviceCache := repository.NewDailyAdviceRepository(db)
	tokens := security.NewTokenManager(cfg.JWTAccessSecret, cfg.JWTRefreshSecret, cfg.AccessTTL, cfg.RefreshTTL)
	calendarCompletions := repository.NewCalendarCompletionRepository(db)
	auth := handlers.NewAuthHandler(users, tokens, cfg, smartTaskQuota, calendarCompletions)
	googleOAuth := handlers.NewGoogleOAuthHandler(cfg, googleLinks, tokens)
	calendarEvents := handlers.NewCalendarEventsHandler(cfg, googleLinks, users, calendarCompletions)
	smartTask := handlers.NewSmartTaskHandler(cfg, googleLinks, smartTaskQuota, users)
	dailyAdvice := handlers.NewDailyAdviceHandler(cfg, googleLinks, users, dailyAdviceCache)
	backlog := handlers.NewBacklogIdeasHandler(cfg, googleLinks, backlogIdeas)
	collabRepo := repository.NewCollabRepository(db)
	collab := handlers.NewCollabHandler(cfg, googleLinks, users, collabRepo)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", handlers.Health)
	mux.HandleFunc("/api/auth/register", auth.Register)
	mux.HandleFunc("/api/auth/login", auth.Login)
	mux.HandleFunc("/api/auth/refresh", auth.Refresh)
	mux.HandleFunc("/api/auth/logout", auth.Logout)
	mux.Handle("/api/auth/me/avatar", middleware.RequireAuth(tokens, http.HandlerFunc(auth.UploadAvatar)))
	mux.Handle("/api/auth/me/promo", middleware.RequireAuth(tokens, http.HandlerFunc(auth.RedeemPromo)))
	mux.Handle("GET /api/auth/me/xp-rank", middleware.RequireAuth(tokens, http.HandlerFunc(auth.MeXpRank)))
	mux.Handle("GET /api/auth/me/stats", middleware.RequireAuth(tokens, http.HandlerFunc(auth.MeStats)))
	mux.Handle("/api/auth/me", middleware.RequireAuth(tokens, http.HandlerFunc(auth.Me)))

	avatarDir := http.Dir(filepath.Join(cfg.UploadDir, "avatars"))
	mux.Handle("/api/static/avatars/", http.StripPrefix("/api/static/avatars/", http.FileServer(avatarDir)))

	mux.HandleFunc("/api/auth/google/start", googleOAuth.Start)
	mux.HandleFunc("/api/auth/google/callback", googleOAuth.Callback)
	mux.Handle("/api/auth/google/status", middleware.RequireAuth(tokens, http.HandlerFunc(googleOAuth.Status)))
	mux.Handle("GET /api/calendar/events", middleware.RequireAuth(tokens, http.HandlerFunc(calendarEvents.ListEvents)))
	mux.Handle("DELETE /api/calendar/events/{id}", middleware.RequireAuth(tokens, http.HandlerFunc(calendarEvents.DeleteEvent)))
	mux.Handle("POST /api/calendar/events/{id}/complete", middleware.RequireAuth(tokens, http.HandlerFunc(calendarEvents.CompleteEvent)))
	mux.Handle("/api/calendar/smart-task", middleware.RequireAuth(tokens, smartTask))
	mux.Handle("GET /api/calendar/daily-advice", middleware.RequireAuth(tokens, dailyAdvice))

	mux.Handle("GET /api/backlog/ideas", middleware.RequireAuth(tokens, http.HandlerFunc(backlog.ListIdeas)))
	mux.Handle("POST /api/backlog/ideas", middleware.RequireAuth(tokens, http.HandlerFunc(backlog.CreateIdea)))
	mux.Handle("DELETE /api/backlog/ideas/{id}", middleware.RequireAuth(tokens, http.HandlerFunc(backlog.DeleteIdea)))
	mux.Handle("POST /api/backlog/ideas/{id}/schedule", middleware.RequireAuth(tokens, http.HandlerFunc(backlog.ScheduleIdea)))

	mux.Handle("POST /api/spaces/join", middleware.RequireAuth(tokens, http.HandlerFunc(collab.JoinSpace)))
	mux.Handle("GET /api/spaces/calendar-overlays", middleware.RequireAuth(tokens, http.HandlerFunc(collab.ListCalendarAcceptedOverlays)))
	mux.Handle("GET /api/spaces/pending-for-me", middleware.RequireAuth(tokens, http.HandlerFunc(collab.ListPendingProposalsForMe)))
	mux.Handle("GET /api/spaces", middleware.RequireAuth(tokens, http.HandlerFunc(collab.ListSpaces)))
	mux.Handle("POST /api/spaces", middleware.RequireAuth(tokens, http.HandlerFunc(collab.CreateSpace)))
	mux.Handle("GET /api/spaces/{id}", middleware.RequireAuth(tokens, http.HandlerFunc(collab.GetSpace)))
	mux.Handle("DELETE /api/spaces/{id}", middleware.RequireAuth(tokens, http.HandlerFunc(collab.DeleteSpace)))
	mux.Handle("GET /api/spaces/{id}/members", middleware.RequireAuth(tokens, http.HandlerFunc(collab.ListMembers)))
	mux.Handle("POST /api/spaces/{id}/invites", middleware.RequireAuth(tokens, http.HandlerFunc(collab.CreateInvite)))
	mux.Handle("GET /api/spaces/{id}/proposals", middleware.RequireAuth(tokens, http.HandlerFunc(collab.ListProposals)))
	mux.Handle("POST /api/spaces/{id}/proposals", middleware.RequireAuth(tokens, http.HandlerFunc(collab.CreateProposal)))
	mux.Handle("POST /api/spaces/{id}/proposals/{proposalId}/respond", middleware.RequireAuth(tokens, http.HandlerFunc(collab.RespondProposal)))

	addr := "127.0.0.1:" + cfg.Port

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, middleware.CORS(cfg.AllowedOrigin, mux)); err != nil {
		log.Fatal(err)
	}
}

func initDB(dsn string) *sql.DB {
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	if err = db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	return db
}
