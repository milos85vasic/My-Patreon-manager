package content

import (
	"context"
	"fmt"

	"github.com/milos85vasic/My-Patreon-Manager/internal/database"
	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type ReviewQueue struct {
	store database.GeneratedContentStore
}

func NewReviewQueue(store database.GeneratedContentStore) *ReviewQueue {
	return &ReviewQueue{store: store}
}

func (rq *ReviewQueue) AddToReview(ctx context.Context, content *models.GeneratedContent) error {
	content.PassedQualityGate = false
	if rq.store != nil {
		return rq.store.Create(ctx, content)
	}
	return nil
}

func (rq *ReviewQueue) ListPending(ctx context.Context) ([]*models.GeneratedContent, error) {
	if rq.store == nil {
		return nil, nil
	}
	return rq.store.GetByQualityRange(ctx, 0, 0.75)
}

func (rq *ReviewQueue) Approve(ctx context.Context, contentID string) error {
	if rq.store == nil {
		return nil
	}
	content, err := rq.store.GetByID(ctx, contentID)
	if err != nil {
		return fmt.Errorf("get content for approval: %w", err)
	}
	if content == nil {
		return fmt.Errorf("content not found: %s", contentID)
	}
	content.PassedQualityGate = true
	return nil
}

func (rq *ReviewQueue) Reject(ctx context.Context, contentID string) error {
	if rq.store == nil {
		return nil
	}
	return nil
}
