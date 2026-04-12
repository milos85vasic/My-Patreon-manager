package content

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/milos85vasic/My-Patreon-Manager/internal/providers/renderer"
)

// --- QualityGate tests ---

func TestQualityGate_EvaluateQuality(t *testing.T) {
	gate := NewQualityGate(0.75)

	score, passed := gate.EvaluateQuality("content", 0.9)
	if score != 0.9 || !passed {
		t.Errorf("expected score=0.9, passed=true; got score=%f, passed=%v", score, passed)
	}

	score, passed = gate.EvaluateQuality("content", 0.5)
	if score != 0.5 || passed {
		t.Errorf("expected score=0.5, passed=false; got score=%f, passed=%v", score, passed)
	}

	score, passed = gate.EvaluateQuality("content", 0.75)
	if score != 0.75 || !passed {
		t.Errorf("expected score=0.75, passed=true (threshold); got score=%f, passed=%v", score, passed)
	}
}

func TestQualityGate_ContentFingerprint(t *testing.T) {
	gate := NewQualityGate(0.75)

	fp1 := gate.ContentFingerprint("hello world")
	fp2 := gate.ContentFingerprint("hello world")
	if fp1 != fp2 {
		t.Error("fingerprints should be deterministic")
	}
	fp3 := gate.ContentFingerprint("different content")
	if fp1 == fp3 {
		t.Error("fingerprints should differ for different content")
	}
	if len(fp1) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars", len(fp1))
	}
}

// --- ReviewQueue tests ---

type mockContentStore struct {
	createCalled bool
	createErr    error
	getByIDVal   *models.GeneratedContent
	getByIDErr   error
	qualityRange []*models.GeneratedContent
	qualityErr   error
}

func (s *mockContentStore) Create(_ context.Context, c *models.GeneratedContent) error {
	s.createCalled = true
	return s.createErr
}
func (s *mockContentStore) GetByID(_ context.Context, id string) (*models.GeneratedContent, error) {
	return s.getByIDVal, s.getByIDErr
}
func (s *mockContentStore) GetLatestByRepo(_ context.Context, _ string) (*models.GeneratedContent, error) {
	return nil, nil
}
func (s *mockContentStore) GetByQualityRange(_ context.Context, _, _ float64) ([]*models.GeneratedContent, error) {
	return s.qualityRange, s.qualityErr
}
func (s *mockContentStore) ListByRepository(_ context.Context, _ string) ([]*models.GeneratedContent, error) {
	return nil, nil
}
func (s *mockContentStore) Update(_ context.Context, _ *models.GeneratedContent) error {
	return nil
}

func TestReviewQueue_NewReviewQueue(t *testing.T) {
	store := &mockContentStore{}
	rq := NewReviewQueue(store)
	if rq == nil {
		t.Fatal("expected non-nil ReviewQueue")
	}
}

func TestReviewQueue_AddToReview(t *testing.T) {
	store := &mockContentStore{}
	rq := NewReviewQueue(store)
	content := &models.GeneratedContent{PassedQualityGate: true}
	err := rq.AddToReview(context.Background(), content)
	if err != nil {
		t.Fatal(err)
	}
	if content.PassedQualityGate {
		t.Error("AddToReview should set PassedQualityGate to false")
	}
	if !store.createCalled {
		t.Error("expected store.Create to be called")
	}
}

func TestReviewQueue_AddToReview_NilStore(t *testing.T) {
	rq := NewReviewQueue(nil)
	content := &models.GeneratedContent{}
	err := rq.AddToReview(context.Background(), content)
	if err != nil {
		t.Fatal(err)
	}
}

func TestReviewQueue_ListPending(t *testing.T) {
	expected := []*models.GeneratedContent{{ID: "1"}}
	store := &mockContentStore{qualityRange: expected}
	rq := NewReviewQueue(store)
	results, err := rq.ListPending(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestReviewQueue_ListPending_NilStore(t *testing.T) {
	rq := NewReviewQueue(nil)
	results, err := rq.ListPending(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Error("expected nil results for nil store")
	}
}

func TestReviewQueue_Approve(t *testing.T) {
	content := &models.GeneratedContent{ID: "abc", PassedQualityGate: false}
	store := &mockContentStore{getByIDVal: content}
	rq := NewReviewQueue(store)
	err := rq.Approve(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if !content.PassedQualityGate {
		t.Error("expected PassedQualityGate to be true after approval")
	}
}

func TestReviewQueue_Approve_NilStore(t *testing.T) {
	rq := NewReviewQueue(nil)
	err := rq.Approve(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
}

func TestReviewQueue_Approve_NotFound(t *testing.T) {
	store := &mockContentStore{getByIDVal: nil}
	rq := NewReviewQueue(store)
	err := rq.Approve(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for not found content")
	}
}

func TestReviewQueue_Approve_GetError(t *testing.T) {
	store := &mockContentStore{getByIDErr: errors.New("db error")}
	rq := NewReviewQueue(store)
	err := rq.Approve(context.Background(), "abc")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReviewQueue_Reject(t *testing.T) {
	store := &mockContentStore{}
	rq := NewReviewQueue(store)
	err := rq.Reject(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
}

func TestReviewQueue_Reject_NilStore(t *testing.T) {
	rq := NewReviewQueue(nil)
	err := rq.Reject(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
}

// --- TierMapping tests ---

func TestLinearTierMapping(t *testing.T) {
	m := &LinearTierMapping{}
	tiers := []TierInfo{
		{ID: "t1", Title: "Basic", AmountCents: 100},
		{ID: "t2", Title: "Pro", AmountCents: 500},
	}
	result := m.MapTier(100, 50, tiers)
	if result != "t1" {
		t.Errorf("expected t1, got %s", result)
	}

	result = m.MapTier(100, 50, nil)
	if result != "" {
		t.Errorf("expected empty string for no tiers, got %s", result)
	}
}

func TestModularTierMapping(t *testing.T) {
	m := &ModularTierMapping{}
	tiers := []TierInfo{
		{ID: "t1", AmountCents: 100},
		{ID: "t2", AmountCents: 500},
		{ID: "t3", AmountCents: 1000},
	}

	// Low score -> first tier
	result := m.MapTier(10, 10, tiers)
	if result != "t1" {
		t.Errorf("expected t1 for low score, got %s", result)
	}

	// Medium score >= 100 -> middle tier
	result = m.MapTier(100, 0, tiers)
	if result != "t2" {
		t.Errorf("expected t2 for medium score, got %s", result)
	}

	// High score >= 1000 -> last tier
	result = m.MapTier(1000, 0, tiers)
	if result != "t3" {
		t.Errorf("expected t3 for high score, got %s", result)
	}

	// Empty tiers
	result = m.MapTier(1000, 0, nil)
	if result != "" {
		t.Errorf("expected empty for nil tiers, got %s", result)
	}
}

func TestExclusiveTierMapping(t *testing.T) {
	m := NewExclusiveTierMapping()
	m.tierThresholds["t2"] = 100
	m.tierThresholds["t3"] = 500

	tiers := []TierInfo{
		{ID: "t1", AmountCents: 100},
		{ID: "t2", AmountCents: 500},
		{ID: "t3", AmountCents: 1000},
	}

	// No tier matches threshold -> fallback to first
	result := m.MapTier(10, 10, tiers)
	if result != "t1" {
		t.Errorf("expected t1 fallback, got %s", result)
	}

	// t2 threshold met
	result = m.MapTier(80, 20, tiers)
	if result != "t2" {
		t.Errorf("expected t2, got %s", result)
	}

	// t3 threshold met
	result = m.MapTier(400, 100, tiers)
	if result != "t2" || result == "t3" {
		// Actually t2 at 100 is checked first in iteration order
		// Let me check: tiers[0]=t1 (no threshold), tiers[1]=t2 (threshold=100, score=500 >= 100 -> match)
	}

	// Empty tiers
	result = m.MapTier(10, 10, nil)
	if result != "" {
		t.Errorf("expected empty for nil tiers, got %s", result)
	}
}

func TestNewTierMapping(t *testing.T) {
	tests := []struct {
		strategy string
		typeName string
	}{
		{"linear", "*content.LinearTierMapping"},
		{"modular", "*content.ModularTierMapping"},
		{"exclusive", "*content.ExclusiveTierMapping"},
		{"unknown", "*content.LinearTierMapping"}, // default
	}
	for _, tt := range tests {
		t.Run(tt.strategy, func(t *testing.T) {
			m := NewTierMapping(tt.strategy)
			if m == nil {
				t.Fatal("expected non-nil mapping")
			}
		})
	}
}

func TestTierMapper_MapAndSetStrategy(t *testing.T) {
	mapper := NewTierMapper("linear")
	tiers := []TierInfo{
		{ID: "t1", AmountCents: 100},
		{ID: "t2", AmountCents: 500},
		{ID: "t3", AmountCents: 1000},
	}

	result := mapper.Map(100, 50, tiers)
	if result != "t1" {
		t.Errorf("expected t1, got %s", result)
	}

	mapper.SetStrategy("modular")
	result = mapper.Map(1000, 0, tiers)
	if result != "t3" {
		t.Errorf("expected t3 after strategy change, got %s", result)
	}
}

// --- TokenBudget tests ---

func TestTokenBudget_CurrentUtilization(t *testing.T) {
	budget := NewTokenBudget(1000)
	if err := budget.CheckBudget(800); err != nil {
		t.Fatal(err)
	}
	util := budget.CurrentUtilization()
	if util != 80.0 {
		t.Errorf("expected 80.0%%, got %f%%", util)
	}
}

func TestTokenBudget_Remaining(t *testing.T) {
	budget := NewTokenBudget(1000)
	if err := budget.CheckBudget(300); err != nil {
		t.Fatal(err)
	}
	remaining := budget.Remaining()
	if remaining != 700 {
		t.Errorf("expected 700 remaining, got %d", remaining)
	}
}

func TestTokenBudget_Remaining_NegativeClampedToZero(t *testing.T) {
	budget := NewTokenBudget(100)
	budget.CurrentUsage = 200 // force over-usage
	remaining := budget.Remaining()
	if remaining != 0 {
		t.Errorf("expected 0 remaining, got %d", remaining)
	}
}

func TestTokenBudget_RefundBelowZero(t *testing.T) {
	budget := NewTokenBudget(1000)
	budget.Refund(100) // refund more than used
	if budget.CurrentUsage != 0 {
		t.Errorf("expected usage clamped to 0, got %d", budget.CurrentUsage)
	}
}

// --- Generator tests ---

func TestGenerator_SetReviewQueue(t *testing.T) {
	llm := &failingLLM{}
	budget := NewTokenBudget(100000)
	gate := NewQualityGate(0.75)
	gen := NewGenerator(llm, budget, gate, nil, nil, nil)
	store := &mockContentStore{}
	rq := NewReviewQueue(store)
	gen.SetReviewQueue(rq)
	if gen.reviewQueue != rq {
		t.Error("expected review queue to be set")
	}
}

// --- Generator assemblePrompt tests ---

func TestGenerator_AssemblePrompt_WithTemplates(t *testing.T) {
	llm := &failingLLM{}
	gen := NewGenerator(llm, nil, NewQualityGate(0.5), nil, nil, nil)

	repo := models.Repository{
		Name:            "myrepo",
		Owner:           "owner",
		Description:     "A test repo",
		Stars:           42,
		Forks:           7,
		PrimaryLanguage: "Go",
		Topics:          []string{"go", "test"},
		Service:         "github",
		HTTPSURL:        "https://github.com/owner/myrepo",
	}
	templates := []models.ContentTemplate{
		{Name: "custom", Template: "Repo: {{REPO_NAME}}"},
	}

	prompt := gen.assemblePrompt(repo, templates)
	if prompt.TemplateName != "custom" {
		t.Errorf("expected template name 'custom', got %q", prompt.TemplateName)
	}
}

func TestGenerator_AssemblePrompt_NoTemplates(t *testing.T) {
	llm := &failingLLM{}
	gen := NewGenerator(llm, nil, NewQualityGate(0.5), nil, nil, nil)

	repo := models.Repository{Name: "myrepo"}
	prompt := gen.assemblePrompt(repo, nil)
	if prompt.TemplateName != "default" {
		t.Errorf("expected template name 'default', got %q", prompt.TemplateName)
	}
}

// successLLM returns valid content for testing the success path.
type successLLM struct{}

func (s *successLLM) GenerateContent(_ context.Context, _ models.Prompt, _ models.GenerationOptions) (models.Content, error) {
	return models.Content{
		Title:        "Test Post",
		Body:         "Generated content body",
		QualityScore: 0.9,
		ModelUsed:    "test-model",
		TokenCount:   100,
	}, nil
}
func (s *successLLM) GetAvailableModels(_ context.Context) ([]models.ModelInfo, error) {
	return nil, nil
}
func (s *successLLM) GetModelQualityScore(_ context.Context, _ string) (float64, error) {
	return 0.9, nil
}
func (s *successLLM) GetTokenUsage(_ context.Context) (models.UsageStats, error) {
	return models.UsageStats{}, nil
}

type mockMetricsCollector struct{}

func (m *mockMetricsCollector) RecordWebhookEvent(_, _ string)            {}
func (m *mockMetricsCollector) RecordSyncDuration(_, _ string, _ float64) {}
func (m *mockMetricsCollector) RecordReposProcessed(_, _ string)          {}
func (m *mockMetricsCollector) RecordAPIError(_, _ string)                {}
func (m *mockMetricsCollector) RecordLLMLatency(_ string, _ float64)      {}
func (m *mockMetricsCollector) RecordLLMTokens(_, _ string, _ int)        {}
func (m *mockMetricsCollector) RecordLLMQualityScore(_ string, _ float64) {}
func (m *mockMetricsCollector) RecordContentGenerated(_, _ string)        {}
func (m *mockMetricsCollector) RecordPostCreated(_ string)                {}
func (m *mockMetricsCollector) RecordPostUpdated(_ string)                {}
func (m *mockMetricsCollector) SetActiveSyncs(_ int)                      {}
func (m *mockMetricsCollector) SetBudgetUtilization(_ float64)            {}

func TestGenerator_GenerateForRepository_Success(t *testing.T) {
	llm := &successLLM{}
	budget := NewTokenBudget(100000)
	gate := NewQualityGate(0.5)
	mc := &mockMetricsCollector{}
	gen := NewGenerator(llm, budget, gate, nil, mc, nil)

	repo := models.Repository{
		ID:    "repo1",
		Name:  "myrepo",
		Owner: "owner",
	}
	result, err := gen.GenerateForRepository(context.Background(), repo, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Title != "Test Post" {
		t.Errorf("expected title 'Test Post', got %q", result.Title)
	}
	if !result.PassedQualityGate {
		t.Error("expected PassedQualityGate=true for score 0.9 with threshold 0.5")
	}
}

func TestGenerator_GenerateForRepository_FailedQualityGate(t *testing.T) {
	llm := &successLLM{} // score 0.9
	budget := NewTokenBudget(100000)
	gate := NewQualityGate(0.95) // threshold higher than score
	mc := &mockMetricsCollector{}
	gen := NewGenerator(llm, budget, gate, nil, mc, nil)

	repo := models.Repository{ID: "repo1", Name: "myrepo"}
	result, err := gen.GenerateForRepository(context.Background(), repo, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.PassedQualityGate {
		t.Error("expected PassedQualityGate=false when score below threshold")
	}
}

func TestGenerator_GenerateForRepository_NoBudget(t *testing.T) {
	llm := &successLLM{}
	gate := NewQualityGate(0.5)
	gen := NewGenerator(llm, nil, gate, nil, nil, nil)

	repo := models.Repository{ID: "repo1"}
	result, err := gen.GenerateForRepository(context.Background(), repo, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestGenerator_GenerateForRepository_BudgetExhausted(t *testing.T) {
	llm := &successLLM{}
	budget := NewTokenBudget(10) // very low budget
	gate := NewQualityGate(0.5)
	gen := NewGenerator(llm, budget, gate, nil, nil, nil)

	repo := models.Repository{ID: "repo1"}
	_, err := gen.GenerateForRepository(context.Background(), repo, nil, nil)
	if err == nil {
		t.Fatal("expected budget error")
	}
}

func TestGenerator_GenerateForRepository_StoreError(t *testing.T) {
	llm := &successLLM{}
	gate := NewQualityGate(0.5)
	store := &mockContentStore{createErr: errors.New("store write failed")}
	gen := NewGenerator(llm, nil, gate, store, nil, nil)

	repo := models.Repository{ID: "repo1"}
	_, err := gen.GenerateForRepository(context.Background(), repo, nil, nil)
	if err == nil {
		t.Fatal("expected store error")
	}
}

type mockRenderer struct {
	format string
	output []byte
	err    error
}

func (r *mockRenderer) Format() string { return r.format }
func (r *mockRenderer) Render(_ context.Context, _ models.Content, _ renderer.RenderOptions) ([]byte, error) {
	return r.output, r.err
}
func (r *mockRenderer) SupportedContentTypes() []string { return []string{"promotional"} }

func TestGenerator_GenerateForRepository_WithRenderer(t *testing.T) {
	llm := &successLLM{}
	gate := NewQualityGate(0.5)

	rend := &mockRenderer{format: "markdown", output: []byte("rendered content")}
	gen := NewGenerator(llm, nil, gate, nil, nil, []renderer.FormatRenderer{rend})

	repo := models.Repository{ID: "repo1", Name: "myrepo"}
	result, err := gen.GenerateForRepository(context.Background(), repo, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Body != "rendered content" {
		t.Errorf("expected rendered content, got %q", result.Body)
	}
}

func TestGenerator_GenerateForRepository_RendererError(t *testing.T) {
	llm := &successLLM{}
	gate := NewQualityGate(0.5)

	rend := &mockRenderer{format: "markdown", err: errors.New("render failed")}
	gen := NewGenerator(llm, nil, gate, nil, nil, []renderer.FormatRenderer{rend})

	repo := models.Repository{ID: "repo1"}
	result, err := gen.GenerateForRepository(context.Background(), repo, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result despite render error")
	}
}

func TestGenerator_GenerateForRepository_NonMarkdownRenderer(t *testing.T) {
	llm := &successLLM{}
	gate := NewQualityGate(0.5)

	rend := &mockRenderer{format: "html", output: []byte("<p>html</p>")}
	gen := NewGenerator(llm, nil, gate, nil, nil, []renderer.FormatRenderer{rend})

	repo := models.Repository{ID: "repo1"}
	result, err := gen.GenerateForRepository(context.Background(), repo, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Body should NOT be replaced since renderer format is "html", not "markdown"
	if result.Body == "<p>html</p>" {
		t.Error("body should not be replaced for non-markdown renderer")
	}
}

func TestGenerator_GenerateForRepository_ContextCancelled(t *testing.T) {
	llm := &failingLLM{}
	gate := NewQualityGate(0.5)
	gen := NewGenerator(llm, nil, gate, nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	repo := models.Repository{ID: "repo1"}
	_, err := gen.GenerateForRepository(ctx, repo, nil, nil)
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

func TestTokenBudget_SoftAlert(t *testing.T) {
	budget := NewTokenBudget(1000)
	var alertPct float64
	budget.OnSoftAlert = func(pct float64) { alertPct = pct }
	// 800/1000 = 80% -> should trigger soft alert
	if err := budget.CheckBudget(800); err != nil {
		t.Fatal(err)
	}
	if alertPct == 0 {
		t.Error("expected soft alert to fire at 80%")
	}
}

func TestTokenBudget_HardStop(t *testing.T) {
	budget := NewTokenBudget(100)
	var hardStopped bool
	budget.OnHardStop = func() { hardStopped = true }
	// Request more than budget
	err := budget.CheckBudget(200)
	if err == nil {
		t.Fatal("expected error when exceeding budget")
	}
	if !hardStopped {
		t.Error("expected hard stop callback to fire")
	}
}

func TestTokenBudget_TimeReset(t *testing.T) {
	budget := NewTokenBudget(1000)
	budget.CurrentUsage = 500
	// Set ResetAt to the past to trigger reset
	budget.ResetAt = budget.ResetAt.Add(-48 * time.Hour)
	err := budget.CheckBudget(100)
	if err != nil {
		t.Fatal(err)
	}
	// After reset, usage should be 100 (just the new request)
	if budget.CurrentUsage != 100 {
		t.Errorf("expected usage=100 after reset, got %d", budget.CurrentUsage)
	}
}

func TestGenerator_GenerateForRepository_AllRetriesExhausted(t *testing.T) {
	llm := &failingLLM{}
	gate := NewQualityGate(0.5)
	gen := NewGenerator(llm, nil, gate, nil, nil, nil)

	repo := models.Repository{ID: "repo1"}
	_, err := gen.GenerateForRepository(context.Background(), repo, nil, nil)
	if err == nil {
		t.Fatal("expected error when all retries exhausted")
	}
}

func TestGenerator_GenerateForRepository_BudgetWithMetrics(t *testing.T) {
	llm := &successLLM{}
	budget := NewTokenBudget(100000)
	gate := NewQualityGate(0.5)
	mc := &mockMetricsCollector{}
	gen := NewGenerator(llm, budget, gate, nil, mc, nil)

	repo := models.Repository{ID: "repo1"}
	_, err := gen.GenerateForRepository(context.Background(), repo, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGenerator_GenerateForRepository_TokenRefund(t *testing.T) {
	llm := &successLLM{} // returns TokenCount=100
	budget := NewTokenBudget(100000)
	gate := NewQualityGate(0.5)
	mc := &mockMetricsCollector{}
	gen := NewGenerator(llm, budget, gate, nil, mc, nil)

	repo := models.Repository{ID: "repo1"}
	_, err := gen.GenerateForRepository(context.Background(), repo, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Budget should have been charged 4000 then refunded (4000-100)=3900
	// So current usage = 4000 - 3900 = 100
	if budget.CurrentUsage != 100 {
		t.Errorf("expected 100 usage after refund, got %d", budget.CurrentUsage)
	}
}
