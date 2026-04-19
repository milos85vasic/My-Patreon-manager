package illustration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	imgprov "github.com/milos85vasic/My-Patreon-Manager/internal/providers/image"
)

type Generator struct {
	providers     *imgprov.FallbackProvider
	store         database.IllustrationStore
	styleLoader   *StyleLoader
	promptBuilder *PromptBuilder
	logger        *slog.Logger
	imageDir      string
}

func NewGenerator(
	providers *imgprov.FallbackProvider,
	store database.IllustrationStore,
	styleLoader *StyleLoader,
	promptBuilder *PromptBuilder,
	logger *slog.Logger,
	imageDir string,
) *Generator {
	return &Generator{
		providers:     providers,
		store:         store,
		styleLoader:   styleLoader,
		promptBuilder: promptBuilder,
		logger:        logger,
		imageDir:      imageDir,
	}
}

// GenerateForRevision is the signature expected by process.IllustrationGenerator.
// It produces an illustration for a content revision and returns the full
// Illustration struct; callers who want a markdown embed tag can format it
// themselves from the returned fields.
//
// The legacy generated_content_id FK is not populated (revisions don't have a
// generated_contents row in the new pipeline). Migration 0008 relaxed that
// column to NULL-able and dropped the UNIQUE index on it, so the field is
// left at its Go zero value here and the store writes SQL NULL (see
// IllustrationStore.Create).
//
// On provider failure the method logs a warning and returns (nil, nil) so the
// pipeline can treat it as "no illustration this run" and proceed.
func (g *Generator) GenerateForRevision(
	ctx context.Context,
	repo *models.Repository,
	body string,
) (*models.Illustration, error) {
	if repo == nil {
		return nil, fmt.Errorf("repo required")
	}
	prompt := g.promptBuilder.BuildFromFields(
		repo.Name, repo.Description,
		repo.PrimaryLanguage, repo.Topics,
		"", body, // no title; the pipeline passes body only
	)
	style := g.styleLoader.LoadStyle(nil)
	fingerprint := computeFingerprint(prompt, style)

	existing, err := g.store.GetByFingerprint(ctx, fingerprint)
	if err == nil && existing != nil && existing.FilePath != "" {
		g.logger.Debug("reusing existing illustration", "fingerprint", fingerprint)
		return existing, nil
	}

	req := imgprov.ImageRequest{
		Prompt:       prompt,
		Style:        style,
		RepositoryID: repo.ID,
	}
	req.SetDefaults()

	result, err := g.providers.GenerateImage(ctx, req)
	if err != nil {
		g.logger.Warn("illustration generation failed, skipping",
			"repository_id", repo.ID, "error", err)
		return nil, nil
	}

	imageData := result.Data
	fileName := fmt.Sprintf("%s.%s", computeContentHash(imageData), result.Format)
	filePath := filepath.Join(g.imageDir, fileName)

	if err := os.MkdirAll(g.imageDir, 0o755); err != nil {
		return nil, fmt.Errorf("create illustration dir: %w", err)
	}
	if len(imageData) > 0 {
		if err := os.WriteFile(filePath, imageData, 0o644); err != nil {
			return nil, fmt.Errorf("write illustration file: %w", err)
		}
	} else if result.URL != "" {
		filePath = result.URL
	}

	ill := &models.Illustration{
		// GeneratedContentID left at the zero value. Migration 0008 made
		// illustrations.generated_content_id nullable; the store writes SQL
		// NULL when this field is empty.
		RepositoryID: repo.ID,
		FilePath:     filePath,
		ImageURL:     result.URL,
		Prompt:       prompt,
		Style:        style,
		ProviderUsed: result.Provider,
		Format:       result.Format,
		ContentHash:  computeContentHash(imageData),
		Fingerprint:  fingerprint,
	}
	ill.GenerateID()
	ill.SetDefaults()

	if err := g.store.Create(ctx, ill); err != nil {
		g.logger.Error("failed to store illustration metadata", "error", err)
		// Best-effort: return the in-memory illustration so the revision at
		// least gets an ID and file path. The missing row in the store is a
		// telemetry loss, not data loss.
	}
	return ill, nil
}

func computeFingerprint(prompt, style string) string {
	h := sha256.Sum256([]byte(prompt + style))
	return hex.EncodeToString(h[:])
}

func computeContentHash(data []byte) string {
	if len(data) == 0 {
		return "no-data"
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])[:16]
}
