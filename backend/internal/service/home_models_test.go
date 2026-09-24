package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type homeModelsSettingRepoStub struct {
	values   map[string]string
	setErr   error
	setCalls int
}

func (r *homeModelsSettingRepoStub) Get(_ context.Context, key string) (*Setting, error) {
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *homeModelsSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := r.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

func (r *homeModelsSettingRepoStub) Set(_ context.Context, key, value string) error {
	r.setCalls++
	if r.setErr != nil {
		return r.setErr
	}
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *homeModelsSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, nil
}

func (r *homeModelsSettingRepoStub) SetMultiple(context.Context, map[string]string) error { return nil }

func (r *homeModelsSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return r.values, nil
}

func (r *homeModelsSettingRepoStub) Delete(context.Context, string) error { return nil }

func homeModelPrice(value float64) *float64 { return &value }

func validHomeTextModel(name string) HomeModel {
	return HomeModel{
		Name:        name,
		Vendor:      "OpenAI",
		Type:        "text",
		Input:       homeModelPrice(1),
		Output:      homeModelPrice(2),
		CachedInput: homeModelPrice(0.1),
		FlexInput:   nil,
	}
}

func TestSettingServiceHomeModels_DefaultRoundTripAndClear(t *testing.T) {
	repo := &homeModelsSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, nil)

	models, err := svc.GetHomeModels(context.Background())
	require.NoError(t, err)
	require.NotNil(t, models)
	require.Empty(t, models)

	toSave := []HomeModel{validHomeTextModel("custom-model")}
	require.NoError(t, svc.SaveHomeModels(context.Background(), toSave))
	require.Equal(t, 1, repo.setCalls)

	reloaded, err := NewSettingService(repo, nil).GetHomeModels(context.Background())
	require.NoError(t, err)
	require.Equal(t, toSave, reloaded)

	require.NoError(t, svc.SaveHomeModels(context.Background(), []HomeModel{}))
	cleared, err := svc.GetHomeModels(context.Background())
	require.NoError(t, err)
	require.NotNil(t, cleared)
	require.Empty(t, cleared)
}

func TestSettingServiceHomeModels_RejectsNullAndInvalidEntries(t *testing.T) {
	repo := &homeModelsSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, nil)

	require.Error(t, svc.SaveHomeModels(context.Background(), nil))
	require.Error(t, svc.SaveHomeModels(context.Background(), []HomeModel{{
		Name:   "missing-prices",
		Vendor: "OpenAI",
		Type:   "text",
	}}))
	require.Error(t, svc.SaveHomeModels(context.Background(), []HomeModel{
		validHomeTextModel("same"),
		validHomeTextModel(" same "),
	}))
	nullImagePrice := map[string]*float64{
		"1K": homeModelPrice(1), "2K": nil, "4K": homeModelPrice(3),
	}
	require.Error(t, svc.SaveHomeModels(context.Background(), []HomeModel{{
		Name: "image", Vendor: "OpenAI", Type: "image", ResolutionPrices: nullImagePrice,
	}}))
	negative := -1.0
	require.Error(t, svc.SaveHomeModels(context.Background(), []HomeModel{{
		Name: "negative", Vendor: "OpenAI", Type: "text", Input: &negative, Output: homeModelPrice(1),
	}}))
}

func TestSettingServiceHomeModels_ReadErrorsAndCorruptDataAreNotDefaulted(t *testing.T) {
	t.Run("read error", func(t *testing.T) {
		repo := &homeModelsSettingRepoStub{values: map[string]string{}}
		readErr := errors.New("database unavailable")
		// 通过 GetValue 的专用包装器保留非 NotFound 错误，确认不会静默使用默认目录。
		svc := NewSettingService(&homeModelsReadErrorRepo{homeModelsSettingRepoStub: repo, err: readErr}, nil)
		_, err := svc.GetHomeModels(context.Background())
		require.ErrorIs(t, err, readErr)
	})

	t.Run("corrupt value", func(t *testing.T) {
		repo := &homeModelsSettingRepoStub{values: map[string]string{SettingKeyHomeModelCatalog: "[] trailing"}}
		_, err := NewSettingService(repo, nil).GetHomeModels(context.Background())
		require.Error(t, err)
		require.NotContains(t, err.Error(), "gpt-5.6-sol")
	})

	t.Run("write error", func(t *testing.T) {
		repo := &homeModelsSettingRepoStub{values: map[string]string{}, setErr: errors.New("write failed")}
		err := NewSettingService(repo, nil).SaveHomeModels(context.Background(), []HomeModel{validHomeTextModel("write-error")})
		require.Error(t, err)
		require.ErrorIs(t, err, repo.setErr)
	})
}

type homeModelsReadErrorRepo struct {
	*homeModelsSettingRepoStub
	err error
}

func (r *homeModelsReadErrorRepo) GetValue(context.Context, string) (string, error) {
	return "", r.err
}
