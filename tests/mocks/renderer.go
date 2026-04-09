package mocks

import (
	"context"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/renderer"
)

type MockFormatRenderer struct {
	FormatFunc                func() string
	RenderFunc                func(ctx context.Context, content models.Content, opts renderer.RenderOptions) ([]byte, error)
	SupportedContentTypesFunc func() []string
}

func (m *MockFormatRenderer) Format() string {
	if m.FormatFunc != nil {
		return m.FormatFunc()
	}
	return "mock"
}

func (m *MockFormatRenderer) Render(ctx context.Context, content models.Content, opts renderer.RenderOptions) ([]byte, error) {
	if m.RenderFunc != nil {
		return m.RenderFunc(ctx, content, opts)
	}
	return nil, nil
}

func (m *MockFormatRenderer) SupportedContentTypes() []string {
	if m.SupportedContentTypesFunc != nil {
		return m.SupportedContentTypesFunc()
	}
	return nil
}
