//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestAccount_OpenAICompatibleHelpersForNvidia(t *testing.T) {
	t.Run("nvidia apikey reuses openai compatible base url and api key", func(t *testing.T) {
		account := &Account{
			Platform: PlatformNvidia,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"api_key": "nvapi-test",
			},
		}

		require.True(t, account.IsOpenAICompatible())
		require.True(t, account.IsOpenAICompatibleAPIKey())
		require.Equal(t, "https://integrate.api.nvidia.com/v1", account.GetOpenAIBaseURL())
		require.Equal(t, "nvapi-test", account.GetOpenAIApiKey())
		require.False(t, account.IsOpenAI())
	})

	t.Run("nvidia custom base url overrides default", func(t *testing.T) {
		account := &Account{
			Platform: PlatformNvidia,
			Type:     AccountTypeAPIKey,
			Credentials: map[string]any{
				"api_key":  "nvapi-test",
				"base_url": "https://custom.nvidia.example/v1",
			},
		}

		require.Equal(t, "https://custom.nvidia.example/v1", account.GetOpenAIBaseURL())
	})

	t.Run("nvidia oauth is not treated as openai compatible apikey", func(t *testing.T) {
		account := &Account{
			Platform: PlatformNvidia,
			Type:     AccountTypeOAuth,
			Credentials: map[string]any{
				"access_token": "token",
			},
		}

		require.True(t, account.IsOpenAICompatible())
		require.False(t, account.IsOpenAICompatibleAPIKey())
		require.Equal(t, "https://integrate.api.nvidia.com/v1", account.GetOpenAIBaseURL())
		require.Empty(t, account.GetOpenAIApiKey())
	})
}

type createAccountRepoStub struct {
	createdAccount *Account
	bindAccountID  int64
	bindGroupIDs   []int64
	nextID         int64
}

func (s *createAccountRepoStub) Create(_ context.Context, account *Account) error {
	clone := *account
	s.createdAccount = &clone
	if s.nextID == 0 {
		s.nextID = 1001
	}
	account.ID = s.nextID
	if s.createdAccount != nil {
		s.createdAccount.ID = s.nextID
	}
	return nil
}

func (s *createAccountRepoStub) BindGroups(_ context.Context, accountID int64, groupIDs []int64) error {
	s.bindAccountID = accountID
	s.bindGroupIDs = append([]int64(nil), groupIDs...)
	return nil
}

func (s *createAccountRepoStub) GetByID(context.Context, int64) (*Account, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) GetByIDs(context.Context, []int64) ([]*Account, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) ExistsByID(context.Context, int64) (bool, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) GetByCRSAccountID(context.Context, string) (*Account, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) FindByExtraField(context.Context, string, any) ([]Account, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) ListCRSAccountIDs(context.Context) (map[string]int64, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) Update(context.Context, *Account) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) Delete(context.Context, int64) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) List(context.Context, pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, string, int64, string) ([]Account, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) ListByGroup(context.Context, int64) ([]Account, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) ListActive(context.Context) ([]Account, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) ListByPlatform(context.Context, string) ([]Account, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) UpdateLastUsed(context.Context, int64) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) BatchUpdateLastUsed(context.Context, map[int64]time.Time) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) SetError(context.Context, int64, string) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) ClearError(context.Context, int64) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) SetSchedulable(context.Context, int64, bool) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) AutoPauseExpiredAccounts(context.Context, time.Time) (int64, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) ListSchedulable(context.Context) ([]Account, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) ListSchedulableByGroupID(context.Context, int64) ([]Account, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) ListSchedulableByPlatform(context.Context, string) ([]Account, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]Account, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) ListSchedulableByPlatforms(context.Context, []string) ([]Account, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) ListSchedulableByGroupIDAndPlatforms(context.Context, int64, []string) ([]Account, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) ListSchedulableUngroupedByPlatform(context.Context, string) ([]Account, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) ListSchedulableUngroupedByPlatforms(context.Context, []string) ([]Account, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) SetRateLimited(context.Context, int64, time.Time) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) SetModelRateLimit(context.Context, int64, string, time.Time) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) SetOverloaded(context.Context, int64, time.Time) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) ClearTempUnschedulable(context.Context, int64) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) ClearRateLimit(context.Context, int64) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) ClearAntigravityQuotaScopes(context.Context, int64) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) ClearModelRateLimits(context.Context, int64) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) UpdateSessionWindow(context.Context, int64, *time.Time, *time.Time, string) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) UpdateExtra(context.Context, int64, map[string]any) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) BulkUpdate(context.Context, []int64, AccountBulkUpdate) (int64, error) {
	panic("unexpected")
}
func (s *createAccountRepoStub) IncrementQuotaUsed(context.Context, int64, float64) error {
	panic("unexpected")
}
func (s *createAccountRepoStub) ResetQuotaUsed(context.Context, int64) error {
	panic("unexpected")
}

type createAccountGroupRepoStub struct {
	groups []Group
}

func (s *createAccountGroupRepoStub) ListActiveByPlatform(_ context.Context, platform string) ([]Group, error) {
	if platform != PlatformOpenAI {
		return nil, nil
	}
	return append([]Group(nil), s.groups...), nil
}

func (s *createAccountGroupRepoStub) Create(context.Context, *Group) error {
	panic("unexpected")
}
func (s *createAccountGroupRepoStub) GetByID(context.Context, int64) (*Group, error) {
	panic("unexpected")
}
func (s *createAccountGroupRepoStub) GetByIDLite(context.Context, int64) (*Group, error) {
	panic("unexpected")
}
func (s *createAccountGroupRepoStub) Update(context.Context, *Group) error {
	panic("unexpected")
}
func (s *createAccountGroupRepoStub) Delete(context.Context, int64) error {
	panic("unexpected")
}
func (s *createAccountGroupRepoStub) DeleteCascade(context.Context, int64) ([]int64, error) {
	panic("unexpected")
}
func (s *createAccountGroupRepoStub) List(context.Context, pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (s *createAccountGroupRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (s *createAccountGroupRepoStub) ListActive(context.Context) ([]Group, error) {
	panic("unexpected")
}
func (s *createAccountGroupRepoStub) ExistsByName(context.Context, string) (bool, error) {
	panic("unexpected")
}
func (s *createAccountGroupRepoStub) GetAccountCount(context.Context, int64) (int64, int64, error) {
	panic("unexpected")
}
func (s *createAccountGroupRepoStub) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected")
}
func (s *createAccountGroupRepoStub) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	panic("unexpected")
}
func (s *createAccountGroupRepoStub) BindAccountsToGroup(context.Context, int64, []int64) error {
	panic("unexpected")
}
func (s *createAccountGroupRepoStub) UpdateSortOrders(context.Context, []GroupSortOrderUpdate) error {
	panic("unexpected")
}

func TestAdminService_CreateAccount_NvidiaBindsOpenAIDefaultGroup(t *testing.T) {
	accountRepo := &createAccountRepoStub{}
	groupRepo := &createAccountGroupRepoStub{
		groups: []Group{
			{ID: 88, Name: "openai-default", Platform: PlatformOpenAI, Status: StatusActive},
		},
	}
	svc := &adminServiceImpl{
		accountRepo: accountRepo,
		groupRepo:   groupRepo,
	}

	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:        "nvidia account",
		Platform:    PlatformNvidia,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "nvapi-test"},
	})

	require.NoError(t, err)
	require.NotNil(t, account)
	require.Equal(t, PlatformNvidia, account.Platform)
	require.Equal(t, int64(1001), account.ID)
	require.Equal(t, int64(1001), accountRepo.bindAccountID)
	require.Equal(t, []int64{88}, accountRepo.bindGroupIDs)
}
