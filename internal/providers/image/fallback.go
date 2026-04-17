package image

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

type FallbackProvider struct {
	providers []ImageProvider
	logger    *slog.Logger
}

func NewFallbackProvider(providers ...ImageProvider) *FallbackProvider {
	return &FallbackProvider{
		providers: providers,
		logger:    slog.Default(),
	}
}

func (f *FallbackProvider) SetLogger(logger *slog.Logger) {
	f.logger = logger
}

func (f *FallbackProvider) ProviderName() string {
	return "fallback"
}

func (f *FallbackProvider) IsAvailable(ctx context.Context) bool {
	for _, p := range f.providers {
		if p.IsAvailable(ctx) {
			return true
		}
	}
	return false
}

func (f *FallbackProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	var errs []string

	for _, p := range f.providers {
		if !p.IsAvailable(ctx) {
			f.logger.Debug("skipping unavailable provider", "provider", p.ProviderName())
			continue
		}

		result, err := p.GenerateImage(ctx, req)
		if err != nil {
			f.logger.Warn("provider failed, trying next",
				"provider", p.ProviderName(),
				"error", err,
			)
			errs = append(errs, fmt.Sprintf("%s: %s", p.ProviderName(), err.Error()))
			continue
		}

		f.logger.Info("illustration generated",
			"provider", p.ProviderName(),
			"repository_id", req.RepositoryID,
		)
		return result, nil
	}

	return nil, fmt.Errorf("all providers failed: %s", strings.Join(errs, "; "))
}
