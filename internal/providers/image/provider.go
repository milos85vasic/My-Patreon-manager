package image

import "context"

type ImageProvider interface {
	GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error)
	ProviderName() string
	IsAvailable(ctx context.Context) bool
}

type ImageRequest struct {
	Prompt       string `json:"prompt"`
	Style        string `json:"style"`
	Size         string `json:"size"`
	Quality      string `json:"quality"`
	Format       string `json:"format"`
	RepositoryID string `json:"repository_id"`
}

func (r *ImageRequest) SetDefaults() {
	if r.Size == "" {
		r.Size = "1792x1024"
	}
	if r.Quality == "" {
		r.Quality = "hd"
	}
	if r.Format == "" {
		r.Format = "png"
	}
}

type ImageResult struct {
	Data     []byte `json:"-"`
	URL      string `json:"url"`
	Format   string `json:"format"`
	Provider string `json:"provider"`
	Prompt   string `json:"prompt"`
}

func (r *ImageResult) HasData() bool {
	return len(r.Data) > 0 || r.URL != ""
}
