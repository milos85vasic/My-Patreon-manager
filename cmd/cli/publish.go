package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/patreon"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/process"
)

// newPublisher is the process.Publisher constructor indirected through a
// package-level variable so tests can swap in a fake without reaching
// into the process package. Matches the dependency-injection pattern
// used elsewhere in cmd/cli.
var newPublisher = func(db database.Database, client process.PatreonMutator) publisher {
	return process.NewPublisher(db, client)
}

// publisher is the narrow interface runPublish needs. The concrete type
// is *process.Publisher; tests substitute a lightweight fake to avoid a
// full DB round trip.
type publisher interface {
	SetLogger(l *slog.Logger)
	PublishPending(ctx context.Context) (int, error)
}

// runPublish executes the "publish" subcommand. It loops through the
// repository queue, checking drift against the live Patreon post and
// pushing the newest approved revision when no drift is found. Per-repo
// failures are logged and skipped; only a catastrophic queue-build
// failure produces a non-zero exit.
//
// The function signature exposes `db` and `patreonClient` so the CLI
// entrypoint can build both once and share them across subcommands; the
// orchestrator is no longer involved in the publish path.
func runPublish(_ context.Context, db database.Database, patreonClient *patreon.Client, logger *slog.Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("received shutdown signal")
		cancel()
	}()

	pub := newPublisher(db, newPatreonMutatorAdapter(patreonClient))
	pub.SetLogger(logger)

	count, err := pub.PublishPending(ctx)
	if err != nil {
		logger.Error("publish failed", slog.String("error", err.Error()))
		osExit(1)
		return
	}
	logger.Info("publish result", slog.Int("published", count))
}

// patreonMutatorAdapter bridges the narrow process.PatreonMutator
// contract onto the fat internal/providers/patreon.Client surface. All
// three methods (GetPostContent, CreatePost, UpdatePost) are wired to
// real Patreon calls; a nil provider client is tolerated and every
// method stubs out (empty body, empty ID, no-op update) so tests and
// misconfigured environments can still drive the publish loop without
// panicking.
type patreonMutatorAdapter struct {
	c *patreon.Client
}

// newPatreonMutatorAdapter returns an adapter bound to the given
// provider client. A nil `c` is tolerated — the adapter simply stubs out
// every method, which keeps main.go's wire-up uniform when the user has
// not configured Patreon credentials.
func newPatreonMutatorAdapter(c *patreon.Client) process.PatreonMutator {
	return &patreonMutatorAdapter{c: c}
}

func (a *patreonMutatorAdapter) GetPostContent(ctx context.Context, postID string) (string, error) {
	if a.c == nil {
		return "", nil
	}
	post, err := a.c.GetPost(ctx, postID)
	if err != nil {
		return "", err
	}
	if post == nil {
		// 404 — caller treats an empty body as "no drift baseline", so
		// a missing post simply bypasses drift detection rather than
		// blocking the publish.
		return "", nil
	}
	return post.Content, nil
}

func (a *patreonMutatorAdapter) CreatePost(ctx context.Context, title, body string, illustrationID *string) (string, error) {
	if a.c == nil {
		return "", nil
	}
	post := &models.Post{
		Title:    title,
		Content:  body,
		PostType: "text_only",
	}
	created, err := a.c.CreatePost(ctx, post)
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

func (a *patreonMutatorAdapter) UpdatePost(ctx context.Context, postID, title, body string, illustrationID *string) error {
	if a.c == nil {
		return nil
	}
	post := &models.Post{
		ID:       postID,
		Title:    title,
		Content:  body,
		PostType: "text_only",
	}
	_, err := a.c.UpdatePost(ctx, post)
	return err
}
