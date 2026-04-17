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

func (g *Generator) Generate(
	ctx context.Context,
	repoID string,
	repoName string,
	repoDesc string,
	repoLang string,
	repoTopics []string,
	contentID string,
	contentTitle string,
	contentBody string,
) (*string, error) {
	prompt := g.promptBuilder.BuildFromFields(repoName, repoDesc, repoLang, repoTopics, contentTitle, contentBody)
	style := g.styleLoader.LoadStyle(nil)
	fingerprint := computeFingerprint(prompt, style)

	existing, err := g.store.GetByFingerprint(ctx, fingerprint)
	if err == nil && existing != nil && existing.FilePath != "" {
		g.logger.Debug("reusing existing illustration", "fingerprint", fingerprint)
		embedTag := fmt.Sprintf("![%s](%s)", contentTitle, existing.FilePath)
		return &embedTag, nil
	}

	req := imgprov.ImageRequest{
		Prompt:       prompt,
		Style:        style,
		RepositoryID: repoID,
	}
	req.SetDefaults()

	result, err := g.providers.GenerateImage(ctx, req)
	if err != nil {
		g.logger.Warn("illustration generation failed, skipping",
			"repository_id", repoID,
			"error", err,
		)
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
		GeneratedContentID: contentID,
		RepositoryID:       repoID,
		FilePath:           filePath,
		ImageURL:           result.URL,
		Prompt:             prompt,
		Style:              style,
		ProviderUsed:       result.Provider,
		Format:             result.Format,
		ContentHash:        computeContentHash(imageData),
		Fingerprint:        fingerprint,
	}
	ill.GenerateID()
	ill.SetDefaults()

	if err := g.store.Create(ctx, ill); err != nil {
		g.logger.Error("failed to store illustration metadata", "error", err)
	}

	embedTag := fmt.Sprintf("![%s](%s)", contentTitle, filePath)
	return &embedTag, nil
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
