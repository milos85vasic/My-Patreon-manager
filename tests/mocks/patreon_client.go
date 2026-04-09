package mocks

import (
	"context"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

type PatreonClient struct {
	GetCampaignFunc      func(ctx context.Context) (models.Campaign, error)
	CreatePostFunc       func(ctx context.Context, post models.Post) (models.Post, error)
	UpdatePostFunc       func(ctx context.Context, post models.Post) (models.Post, error)
	DeletePostFunc       func(ctx context.Context, postID string) error
	ListTiersFunc        func(ctx context.Context, campaignID string) ([]models.Tier, error)
	AssociateTiersFunc   func(ctx context.Context, postID string, tierIDs []string) error
	RefreshTokenFunc     func(ctx context.Context) error
	VerifyMembershipFunc func(ctx context.Context, patronID, campaignID string) ([]models.Tier, error)
}

func (m *PatreonClient) GetCampaign(ctx context.Context) (models.Campaign, error) {
	if m.GetCampaignFunc != nil {
		return m.GetCampaignFunc(ctx)
	}
	return models.Campaign{}, nil
}

func (m *PatreonClient) CreatePost(ctx context.Context, post models.Post) (models.Post, error) {
	if m.CreatePostFunc != nil {
		return m.CreatePostFunc(ctx, post)
	}
	return models.Post{}, nil
}

func (m *PatreonClient) UpdatePost(ctx context.Context, post models.Post) (models.Post, error) {
	if m.UpdatePostFunc != nil {
		return m.UpdatePostFunc(ctx, post)
	}
	return models.Post{}, nil
}

func (m *PatreonClient) DeletePost(ctx context.Context, postID string) error {
	if m.DeletePostFunc != nil {
		return m.DeletePostFunc(ctx, postID)
	}
	return nil
}

func (m *PatreonClient) ListTiers(ctx context.Context, campaignID string) ([]models.Tier, error) {
	if m.ListTiersFunc != nil {
		return m.ListTiersFunc(ctx, campaignID)
	}
	return nil, nil
}

func (m *PatreonClient) AssociateTiers(ctx context.Context, postID string, tierIDs []string) error {
	if m.AssociateTiersFunc != nil {
		return m.AssociateTiersFunc(ctx, postID, tierIDs)
	}
	return nil
}

func (m *PatreonClient) RefreshToken(ctx context.Context) error {
	if m.RefreshTokenFunc != nil {
		return m.RefreshTokenFunc(ctx)
	}
	return nil
}

func (m *PatreonClient) VerifyMembership(ctx context.Context, patronID, campaignID string) ([]models.Tier, error) {
	if m.VerifyMembershipFunc != nil {
		return m.VerifyMembershipFunc(ctx, patronID, campaignID)
	}
	return nil, nil
}
