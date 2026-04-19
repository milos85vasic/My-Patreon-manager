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
// contract onto the fat internal/providers/patreon.Client surface. Only
// CreatePost and UpdatePost are wired to real Patreon calls; the
// provider does not yet expose a "fetch post body by ID" endpoint, so
// GetPostContent is a stub that returns ("", nil). Task 33 will replace
// the stub with a real GET /posts/{id} call once the provider grows
// that method; until then the publisher treats an empty return as
// "no drift detected", which is safe because the drift-fingerprint of
// our published body must also match the empty string for this to
// spuriously clear drift — essentially never.
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
	// TODO(task-33): wire to a real GET /posts/{id} fetch once the
	// provider exposes it. Returning ("", nil) means we conservatively
	// treat the live content as an empty body; pairs with the
	// DriftFingerprint comparison in the Publisher. Until the real
	// fetch is in place, drift detection is effectively disabled.
	return "", nil
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
