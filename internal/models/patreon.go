package models

type Campaign struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Summary     string `json:"summary"`
	CreatorName string `json:"creator_name"`
}

type Post struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	PostType    string `json:"post_type"`
	PublishedAt string `json:"published_at"`
}

type Tier struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	AmountCents float64 `json:"amount_cents"`
}
