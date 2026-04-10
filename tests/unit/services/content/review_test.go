package content_test

import (
	"context"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/services/content"
	"github.com/stretchr/testify/assert"
)

type mockContentStore struct {
	contents []*models.GeneratedContent
}

func (m *mockContentStore) Create(_ context.Context, c *models.GeneratedContent) error {
	m.contents = append(m.contents, c)
	return nil
}
func (m *mockContentStore) GetByID(_ context.Context, id string) (*models.GeneratedContent, error) {
	for _, c := range m.contents {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, nil
}
func (m *mockContentStore) GetLatestByRepo(_ context.Context, _ string) (*models.GeneratedContent, error) {
	return nil, nil
}
func (m *mockContentStore) GetByQualityRange(_ context.Context, min, max float64) ([]*models.GeneratedContent, error) {
	var result []*models.GeneratedContent
	for _, c := range m.contents {
		if c.QualityScore >= min && c.QualityScore <= max {
			result = append(result, c)
		}
	}
	return result, nil
}
func (m *mockContentStore) ListByRepository(_ context.Context, _ string) ([]*models.GeneratedContent, error) {
	return nil, nil
}
func (m *mockContentStore) Update(_ context.Context, c *models.GeneratedContent) error {
	for i, existing := range m.contents {
		if existing.ID == c.ID {
			m.contents[i] = c
			return nil
		}
	}
	return nil
}

func TestReviewQueue_AddToReview(t *testing.T) {
	store := &mockContentStore{}
	rq := content.NewReviewQueue(store)

	c := &models.GeneratedContent{ID: "test-1", QualityScore: 0.5}
	err := rq.AddToReview(context.Background(), c)
	assert.NoError(t, err)
	assert.False(t, c.PassedQualityGate)
}

func TestReviewQueue_ListPending(t *testing.T) {
	store := &mockContentStore{
		contents: []*models.GeneratedContent{
			{ID: "1", QualityScore: 0.3},
			{ID: "2", QualityScore: 0.6},
			{ID: "3", QualityScore: 0.9},
		},
	}

	rq := content.NewReviewQueue(store)
	pending, err := rq.ListPending(context.Background())
	assert.NoError(t, err)
	assert.Len(t, pending, 2)
}

func TestReviewQueue_Approve(t *testing.T) {
	store := &mockContentStore{
		contents: []*models.GeneratedContent{
			{ID: "1", QualityScore: 0.5, PassedQualityGate: false},
		},
	}

	rq := content.NewReviewQueue(store)
	err := rq.Approve(context.Background(), "1")
	assert.NoError(t, err)
	assert.True(t, store.contents[0].PassedQualityGate)
}

func TestReviewQueue_Approve_NotFound(t *testing.T) {
	store := &mockContentStore{}
	rq := content.NewReviewQueue(store)
	err := rq.Approve(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "content not found")
}

func TestReviewQueue_Reject(t *testing.T) {
	store := &mockContentStore{
		contents: []*models.GeneratedContent{
			{ID: "1", QualityScore: 0.5, PassedQualityGate: false},
		},
	}

	rq := content.NewReviewQueue(store)
	err := rq.Reject(context.Background(), "1")
	assert.NoError(t, err)
}

func TestReviewQueue_NilStore(t *testing.T) {
	rq := content.NewReviewQueue(nil)

	err := rq.AddToReview(context.Background(), &models.GeneratedContent{})
	assert.NoError(t, err)

	pending, err := rq.ListPending(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, pending)
}
