package mocks

import (
	"context"

	imgprov "github.com/milos85vasic/My-Patreon-Manager/internal/providers/image"
)

type MockImageProvider struct {
	GenerateImageFunc func(ctx context.Context, req imgprov.ImageRequest) (*imgprov.ImageResult, error)
	ProviderNameFunc  func() string
	IsAvailableFunc   func(ctx context.Context) bool
}

func (m *MockImageProvider) GenerateImage(ctx context.Context, req imgprov.ImageRequest) (*imgprov.ImageResult, error) {
	if m.GenerateImageFunc != nil {
		return m.GenerateImageFunc(ctx, req)
	}
	return nil, nil
}

func (m *MockImageProvider) ProviderName() string {
	if m.ProviderNameFunc != nil {
		return m.ProviderNameFunc()
	}
	return "mock"
}

func (m *MockImageProvider) IsAvailable(ctx context.Context) bool {
	if m.IsAvailableFunc != nil {
		return m.IsAvailableFunc(ctx)
	}
	return true
}
