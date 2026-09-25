package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type communityTestRepository struct {
	CommunityRepository
	member               *CommunityMembership
	challenge            *CommunityChallenge
	invite               *CommunityInvite
	active               bool
	paid                 bool
	paidErr              error
	paidChecks           int
	stateReads           int
	queued               int
	claimedBot           int64
	confirmedBot         int64
	authorizedBot        int64
	marked               []string
	revocations          []CommunityInvite
	completedRevocations int
	failedRevocations    int
	listGroupID          int64
	listBotID            int64
	listFilter           CommunityMemberFilter
}

func (r *communityTestRepository) Cleanup(context.Context) error { return nil }

func (r *communityTestRepository) EnsureActiveUser(context.Context, int64) error {
	if !r.active {
		return ErrCommunityNotFound
	}
	return nil
}
func (r *communityTestRepository) HasPaidBalanceRecharge(context.Context, int64) (bool, error) {
	r.paidChecks++
	return r.paid, r.paidErr
}
func (r *communityTestRepository) GetState(context.Context, int64) (*CommunityMembership, *CommunityChallenge, *CommunityInvite, error) {
	r.stateReads++
	return r.member, r.challenge, r.invite, nil
}
func (r *communityTestRepository) ListMembers(_ context.Context, groupID, botID int64, filter CommunityMemberFilter) (*CommunityMemberPage, error) {
	r.listGroupID = groupID
	r.listBotID = botID
	r.listFilter = filter
	return &CommunityMemberPage{Items: []CommunityMemberItem{}, Page: filter.Page, PageSize: filter.PageSize, Summary: CommunityMemberSummary{Total: 3, Joined: 1, NotJoined: 2}}, nil
}
func (r *communityTestRepository) CreateChallenge(_ context.Context, c *CommunityChallenge) error {
	r.challenge = c
	return nil
}
func (r *communityTestRepository) ClaimChallenge(_ context.Context, hash string, botID int64, identity CommunityTelegramIdentity) (*CommunityChallenge, error) {
	r.claimedBot = botID
	if r.challenge == nil || r.challenge.TokenHash != hash || r.challenge.BotID != botID {
		return nil, ErrCommunityNotFound
	}
	r.challenge.TelegramUserID = identity.ID
	r.challenge.TelegramUsername = identity.Username
	r.challenge.TelegramName = identity.Name
	r.challenge.Status = "claimed"
	return r.challenge, nil
}
func (r *communityTestRepository) ConfirmChallenge(_ context.Context, userID int64, id string, telegramID, groupID, botID int64) (*CommunityMembership, error) {
	r.confirmedBot = botID
	if r.challenge == nil || r.challenge.ID != id || r.challenge.TelegramUserID != telegramID || r.challenge.BotID != botID {
		return nil, ErrCommunityConflict
	}
	r.challenge.Status = "confirmed"
	r.member = &CommunityMembership{UserID: userID, TelegramUserID: telegramID, TelegramUsername: r.challenge.TelegramUsername, GroupChatID: groupID, Status: "pending"}
	return r.member, nil
}
func (r *communityTestRepository) GetMembershipByTelegram(_ context.Context, id int64) (*CommunityMembership, error) {
	if r.member == nil || r.member.TelegramUserID != id {
		return nil, ErrCommunityNotFound
	}
	return r.member, nil
}
func (r *communityTestRepository) AcquireInviteLease(context.Context, int64, int64, string, time.Duration) (bool, error) {
	return true, nil
}
func (r *communityTestRepository) SaveInvite(_ context.Context, invite *CommunityInvite, _ string) error {
	invite.ID = 10
	r.invite = invite
	if r.member != nil {
		r.member.Status = "pending"
		r.member.GroupChatID = invite.GroupChatID
	}
	return nil
}
func (r *communityTestRepository) ReleaseInviteLease(context.Context, int64, string) error {
	return nil
}
func (r *communityTestRepository) AuthorizeJoin(ctx context.Context, hash string, identity CommunityTelegramIdentity, groupID, eventDate, updateID, botID int64, requirePaidRecharge bool) (*CommunityMembership, *CommunityInvite, error) {
	r.authorizedBot = botID
	if !r.active || r.invite == nil || r.invite.URLHash != hash || (r.invite.TelegramUserID != 0 && r.invite.TelegramUserID != identity.ID) || r.invite.GroupChatID != groupID || r.invite.BotID != botID || !r.invite.ExpiresAt.After(time.Now()) {
		return nil, nil, ErrCommunityNotFound
	}
	if r.member != nil && r.member.TelegramUserID != identity.ID {
		return nil, nil, ErrCommunityConflict
	}
	if requirePaidRecharge {
		paid, err := r.HasPaidBalanceRecharge(ctx, r.invite.UserID)
		if err != nil {
			return nil, nil, err
		}
		if !paid {
			return nil, nil, ErrCommunityVIPRequired
		}
	}
	if r.member == nil {
		r.member = &CommunityMembership{UserID: r.invite.UserID, TelegramUserID: identity.ID, TelegramUsername: identity.Username, TelegramName: identity.Name, GroupChatID: groupID, Status: "pending"}
	}
	r.invite.TelegramUserID = identity.ID
	r.member.AuthorizedInviteID = r.invite.ID
	return r.member, r.invite, nil
}
func (r *communityTestRepository) MarkMembership(_ context.Context, telegramID, groupID int64, status string, eventDate, updateID int64) error {
	if r.member == nil || r.member.TelegramUserID != telegramID {
		return ErrCommunityNotFound
	}
	if eventDate < r.member.LastEventDate || (eventDate == r.member.LastEventDate && updateID < r.member.LastUpdateID) {
		return ErrCommunityConflict
	}
	r.marked = append(r.marked, status)
	r.member.Status = status
	if r.invite != nil {
		r.invite.Status = "revoke_pending"
	}
	r.member.LastEventDate = eventDate
	r.member.LastUpdateID = updateID
	if status == "left" {
		r.member.AuthorizedInviteID = 0
		if r.member.JoinedAt == nil {
			r.member = nil
		}
	} else if r.member.JoinedAt == nil {
		now := time.Now()
		r.member.JoinedAt = &now
	}
	return nil
}
func (r *communityTestRepository) QueueInviteRevocation(context.Context, int64, int64) error {
	if r.invite != nil {
		r.invite.Status = "revoke_pending"
	}
	return nil
}
func (r *communityTestRepository) EnqueueWebhook(context.Context, int64, int64, []byte) error {
	r.queued++
	return nil
}
func (r *communityTestRepository) ClaimWebhooks(context.Context, int, time.Duration) ([]CommunityWebhookEvent, error) {
	return nil, nil
}
func (r *communityTestRepository) ClaimRevocations(context.Context, int, time.Duration) ([]CommunityInvite, error) {
	return r.revocations, nil
}
func (r *communityTestRepository) CompleteRevocation(context.Context, int64, string) error {
	r.completedRevocations++
	return nil
}
func (r *communityTestRepository) FailRevocation(context.Context, int64, string, time.Time) error {
	r.failedRevocations++
	return nil
}

type communityTestTelegram struct {
	repo         *communityTestRepository
	methods      []string
	present      bool
	rejected     bool
	botID        int64
	memberStatus string
	publicGroup  bool
	badMember    bool
	sendStatus   int
	messages     []supportTelegramTextRequest
	beforeMember func()
}

func (r *communityTestTelegram) RoundTrip(request *http.Request) (*http.Response, error) {
	method := request.URL.Path[strings.LastIndex(request.URL.Path, "/")+1:]
	r.methods = append(r.methods, method)
	var result any = true
	status := 200
	switch method {
	case "getMe":
		result = communityTelegramUser{ID: r.botID, IsBot: true, Username: "site_test_bot"}
	case "getChat":
		chat := communityTelegramChat{ID: -100, Type: "supergroup", Title: "社群"}
		if r.publicGroup {
			chat.ActiveUsernames = []string{"public_group"}
		}
		result = chat
	case "getChatMember":
		if r.beforeMember != nil {
			r.beforeMember()
		}
		if r.badMember {
			status = 400
		}
		var input communityChatMemberRequest
		_ = json.NewDecoder(request.Body).Decode(&input)
		member := communityTelegramMember{User: communityTelegramUser{ID: input.UserID}, Status: "left"}
		if input.UserID == r.botID {
			member.Status = "administrator"
			member.CanInviteUsers = true
			member.CanRestrictMembers = true
		} else if r.memberStatus != "" {
			member.Status = r.memberStatus
		} else if r.present {
			member.Status = "member"
		}
		result = member
	case "sendMessage":
		var input supportTelegramTextRequest
		_ = json.NewDecoder(request.Body).Decode(&input)
		r.messages = append(r.messages, input)
		if r.sendStatus != 0 {
			status = r.sendStatus
		}
		result = SupportTelegramMessage{MessageID: 1}
	case "createChatInviteLink":
		result = communityTelegramInvite{InviteLink: "https://t.me/+personal_link", CreatesJoinRequest: true, ExpireDate: time.Now().Add(15 * time.Minute).Unix()}
	case "approveChatJoinRequest":
		if r.repo.member == nil || r.repo.member.AuthorizedInviteID == 0 {
			panic("必须先持久化授权再调用批准")
		}
		if r.rejected {
			status = 400
		} else {
			r.present = true
		}
	}
	raw, _ := json.Marshal(map[string]any{"ok": status == 200, "result": result})
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(raw))), Header: make(http.Header)}, nil
}

func communityTestService(t *testing.T) (*CommunityService, *communityTestRepository, *communityTestTelegram) {
	t.Helper()
	settings := newNotificationEmailMemorySettingRepo()
	shared, _ := json.Marshal(SupportDeliverySettings{Enabled: false, TelegramBotToken: "77:abcdefghijklmnopqrstuvwxy", TelegramWebhookSecret: "secret_12345678901234567890"})
	require.NoError(t, settings.Set(context.Background(), SettingKeySupportDelivery, string(shared)))
	c, _ := json.Marshal(CommunitySettings{Enabled: true, ContactURL: "https://example.com/contact", GroupChatID: "-100", GroupName: "社群", BotUsername: "site_test_bot", BotID: 77})
	require.NoError(t, settings.Set(context.Background(), SettingKeyCommunity, string(c)))
	repo := &communityTestRepository{active: true}
	transport := &communityTestTelegram{repo: repo, botID: 77}
	delivery := NewSupportDeliveryService(settings, nil, nil, nil)
	delivery.client.Transport = transport
	return NewCommunityService(settings, repo, delivery), repo, transport
}

func TestCommunityLegacyVerificationRemainsCompatible(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	ctx := context.Background()
	state, err := s.StartVerification(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, "waiting", state.Challenge.Status)
	u, err := url.Parse(state.Challenge.BotURL)
	require.NoError(t, err)
	parameter := u.Query().Get("start")
	require.LessOrEqual(t, len(parameter), 64)
	require.Len(t, repo.challenge.TokenHash, 64)
	require.NotContains(t, repo.challenge.TokenHash, strings.TrimPrefix(parameter, "join_"))
	c, shared, err := s.checkedConfiguration(ctx)
	require.NoError(t, err)
	require.NoError(t, s.processStart(ctx, c, shared, &communityTelegramMessage{Chat: communityTelegramChat{ID: 42, Type: "private"}, From: &communityTelegramUser{ID: 42, Username: "member42", FirstName: "成员"}, Text: "/start " + parameter}))
	require.Nil(t, repo.member)
	require.Equal(t, "claimed", repo.challenge.Status)
	require.EqualValues(t, 77, repo.claimedBot)
	_, err = s.CreateInvite(ctx, 1, CommunityInviteInput{ChallengeID: repo.challenge.ID, TelegramUserID: 99})
	require.ErrorIs(t, err, ErrCommunityConflict)
	require.NotContains(t, telegram.methods, "createChatInviteLink")
	state, err = s.CreateInvite(ctx, 1, CommunityInviteInput{ChallengeID: repo.challenge.ID, TelegramUserID: 42})
	require.NoError(t, err)
	require.Equal(t, "pending", state.Membership.Status)
	require.NotNil(t, state.Invite)
	require.NotContains(t, telegram.methods, "approveChatJoinRequest")
	request := &communityTelegramJoinRequest{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 42}, Date: time.Now().Unix(), InviteLink: &communityTelegramInvite{InviteLink: repo.invite.URL}}
	require.NoError(t, s.processJoinRequest(ctx, c, shared, 12, request))
	require.Equal(t, []string{"joined"}, repo.marked)
	require.Equal(t, "joined", repo.member.Status)
	require.Equal(t, "revoke_pending", repo.invite.Status)
	require.EqualValues(t, 77, repo.authorizedBot)
}

func TestCommunityClaimedInviteCannotReplaceExistingTelegramIdentity(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "pending"}
	repo.invite = &CommunityInvite{ID: 10, BotID: 77, UserID: 1, TelegramUserID: 42, GroupChatID: -100, URL: "https://t.me/+personal_link", URLHash: communityHash("https://t.me/+personal_link"), ExpiresAt: time.Now().Add(time.Minute), Status: "active"}
	c, shared, err := s.checkedConfiguration(context.Background())
	require.NoError(t, err)
	require.NoError(t, s.processJoinRequest(context.Background(), c, shared, 14, &communityTelegramJoinRequest{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 99}, Date: time.Now().Unix(), InviteLink: &communityTelegramInvite{InviteLink: repo.invite.URL}}))
	require.Contains(t, telegram.methods, "declineChatJoinRequest")
	require.NotContains(t, telegram.methods, "approveChatJoinRequest")
	require.Empty(t, repo.marked)
}

func TestCommunityWebhookPersistsOnlyAuthenticatedRelevantEvents(t *testing.T) {
	s, repo, _ := communityTestService(t)
	payload := []byte(`{"update_id":5,"chat_join_request":{"chat":{"id":-100},"from":{"id":42}}}`)
	require.ErrorIs(t, s.HandleTelegramWebhook(context.Background(), "wrong", payload), ErrSupportTelegramUnauthorized)
	require.Zero(t, repo.queued)
	require.NoError(t, s.HandleTelegramWebhook(context.Background(), "secret_12345678901234567890", payload))
	require.Equal(t, 1, repo.queued)
	require.NoError(t, s.HandleTelegramWebhook(context.Background(), "secret_12345678901234567890", []byte(`{"update_id":6,"message":{"chat":{"id":-100,"type":"supergroup"},"text":"/start join_other"}}`)))
	require.Equal(t, 1, repo.queued)
}

func TestCommunityUnverifiedDirectMemberIsRemovedButAdministratorIsPreserved(t *testing.T) {
	for _, status := range []string{"member", "administrator", "creator"} {
		t.Run(status, func(t *testing.T) {
			s, _, telegram := communityTestService(t)
			telegram.memberStatus = status
			c, shared, err := s.checkedConfiguration(context.Background())
			require.NoError(t, err)
			require.NoError(t, s.processMemberUpdate(context.Background(), c, shared, 10, &communityTelegramMemberUpdate{Chat: communityTelegramChat{ID: -100}, Date: time.Now().Unix(), NewChatMember: communityTelegramMember{User: communityTelegramUser{ID: 99}, Status: "member"}}))
			if status == "member" {
				require.Contains(t, telegram.methods, "banChatMember")
			} else {
				require.NotContains(t, telegram.methods, "banChatMember")
			}
		})
	}
}

func TestCommunityStaleJoinCannotKickJoinedMemberOrUndoLaterLeave(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "joined", LastEventDate: 200, LastUpdateID: 20}
	c, shared, err := s.checkedConfiguration(context.Background())
	require.NoError(t, err)
	require.NoError(t, s.processJoinRequest(context.Background(), c, shared, 10, &communityTelegramJoinRequest{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 42}, Date: 100}))
	require.NotContains(t, telegram.methods, "banChatMember")
	require.NotContains(t, telegram.methods, "approveChatJoinRequest")
	repo.member.Status = "left"
	require.NoError(t, s.processMemberUpdate(context.Background(), c, shared, 10, &communityTelegramMemberUpdate{Chat: communityTelegramChat{ID: -100}, Date: 100, NewChatMember: communityTelegramMember{User: communityTelegramUser{ID: 42}, Status: "member"}}))
	require.Equal(t, "left", repo.member.Status)
	require.Empty(t, repo.marked)
}

func TestCommunityChangingBotStopsOldTokenTasks(t *testing.T) {
	s, _, telegram := communityTestService(t)
	telegram.botID = 78
	_, _, err := s.checkedConfiguration(context.Background())
	require.ErrorIs(t, err, ErrCommunityDisabled)
	require.Equal(t, []string{"getMe"}, telegram.methods)
}

func TestCommunitySettingsRejectPublicGroupsAndAllowOfflineContact(t *testing.T) {
	s, _, telegram := communityTestService(t)
	telegram.publicGroup = true
	_, err := s.UpdateSettings(context.Background(), CommunitySettings{Enabled: true, GroupChatID: "-100"})
	require.ErrorIs(t, err, ErrCommunityInvalid)
	before := len(telegram.methods)
	c, err := s.UpdateSettings(context.Background(), CommunitySettings{Enabled: false, ContactURL: "https://example.com/help", BotID: 999})
	require.NoError(t, err)
	require.EqualValues(t, 77, c.BotID)
	require.Len(t, telegram.methods, before)
	state, err := s.Get(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/help", state.ContactURL)
	require.False(t, state.Enabled)
	raw, err := json.Marshal(state)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"membership":null`)
	require.Contains(t, string(raw), `"challenge":null`)
	require.Contains(t, string(raw), `"invite":null`)
}

func TestCommunityCancelledJoinClearsPendingAuthorization(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	telegram.rejected = true
	repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "pending"}
	repo.invite = &CommunityInvite{ID: 10, BotID: 77, UserID: 1, TelegramUserID: 42, GroupChatID: -100, URL: "https://t.me/+personal_link", URLHash: communityHash("https://t.me/+personal_link"), ExpiresAt: time.Now().Add(time.Minute), Status: "active"}
	c, shared, err := s.checkedConfiguration(context.Background())
	require.NoError(t, err)
	require.NoError(t, s.processJoinRequest(context.Background(), c, shared, 30, &communityTelegramJoinRequest{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 42}, Date: time.Now().Unix(), InviteLink: &communityTelegramInvite{InviteLink: repo.invite.URL}}))
	require.Nil(t, repo.member)
}

func TestCommunityRevocationDoesNotSwallowMemberLookupFailure(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	telegram.badMember = true
	repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "pending", AuthorizedInviteID: 10}
	repo.revocations = []CommunityInvite{{ID: 10, BotID: 77, UserID: 1, TelegramUserID: 42, GroupChatID: -100, URL: "https://t.me/+personal_link", ExpiresAt: time.Now().Add(-time.Minute)}}
	s.process(context.Background())
	require.Equal(t, 1, repo.failedRevocations)
	require.Zero(t, repo.completedRevocations)
	require.NotContains(t, telegram.methods, "revokeChatInviteLink")
	require.Equal(t, "pending", repo.member.Status)
}

func TestCommunityPrivateAcknowledgment403DoesNotRetryClaim(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	telegram.sendStatus = 403
	token := strings.Repeat("a", 43)
	repo.challenge = &CommunityChallenge{ID: "challenge", BotID: 77, TokenHash: communityHash(token), Status: "waiting"}
	c, shared, err := s.checkedConfiguration(context.Background())
	require.NoError(t, err)
	require.NoError(t, s.processStart(context.Background(), c, shared, &communityTelegramMessage{Chat: communityTelegramChat{ID: 42, Type: "private"}, From: &communityTelegramUser{ID: 42}, Text: "/start join_" + token}))
	require.Equal(t, "claimed", repo.challenge.Status)
}

func TestCommunityExpiredInviteReconcilesInactiveWebsiteMember(t *testing.T) {
	for _, status := range []string{"member", "administrator"} {
		t.Run(status, func(t *testing.T) {
			s, repo, telegram := communityTestService(t)
			repo.active = false
			repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "pending", AuthorizedInviteID: 10}
			telegram.memberStatus = status
			c, shared, err := s.checkedConfiguration(context.Background())
			require.NoError(t, err)
			err = s.reconcileExpiringInvite(context.Background(), c, shared, CommunityInvite{ID: 10, BotID: 77, UserID: 1, TelegramUserID: 42, GroupChatID: -100, ExpiresAt: time.Now().Add(-time.Minute)})
			require.NoError(t, err)
			require.Nil(t, repo.member)
			if status == "member" {
				require.Contains(t, telegram.methods, "banChatMember")
			} else {
				require.NotContains(t, telegram.methods, "banChatMember")
			}
		})
	}
}

func TestCommunityDirectInviteNeedsNoTelegramVerificationAndBindsFirstJoiner(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	ctx := context.Background()
	state, err := s.CreateInvite(ctx, 1, CommunityInviteInput{})
	require.NoError(t, err)
	require.NotNil(t, state.Invite)
	require.Nil(t, state.Membership)
	require.Nil(t, repo.challenge)
	require.Zero(t, repo.invite.TelegramUserID)
	require.NotContains(t, telegram.methods, "sendMessage")
	require.NotContains(t, telegram.methods, "approveChatJoinRequest")
	firstURL := state.Invite.URL
	second, err := s.CreateInvite(ctx, 1, CommunityInviteInput{})
	require.NoError(t, err)
	require.Equal(t, firstURL, second.Invite.URL)
	calls := 0
	for _, method := range telegram.methods {
		if method == "createChatInviteLink" {
			calls++
		}
	}
	require.Equal(t, 1, calls)
	c, shared, err := s.checkedConfiguration(ctx)
	require.NoError(t, err)
	request := &communityTelegramJoinRequest{Chat: communityTelegramChat{ID: -100}, From: communityTelegramUser{ID: 99, Username: "first_joiner", FirstName: "首位", LastName: "加入者"}, Date: time.Now().Unix(), InviteLink: &communityTelegramInvite{InviteLink: firstURL}}
	require.NoError(t, s.processJoinRequest(ctx, c, shared, 60, request))
	require.EqualValues(t, 1, repo.member.UserID)
	require.EqualValues(t, 99, repo.member.TelegramUserID)
	require.Equal(t, "first_joiner", repo.member.TelegramUsername)
	require.Equal(t, "joined", repo.member.Status)
	require.NotNil(t, repo.member.JoinedAt)
	request.From.ID = 100
	require.NoError(t, s.processJoinRequest(ctx, c, shared, 61, request))
	require.EqualValues(t, 99, repo.member.TelegramUserID)
	require.Contains(t, telegram.methods, "declineChatJoinRequest")
}

func TestCommunityDirectInviteRejectsInactiveWebsiteUser(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	repo.active = false
	_, err := s.CreateInvite(context.Background(), 1, CommunityInviteInput{})
	require.ErrorIs(t, err, ErrCommunityNotFound)
	require.Empty(t, telegram.methods)
}

func TestCommunityExpiredLegacyReservationCanStartDirectInvite(t *testing.T) {
	s, repo, _ := communityTestService(t)
	repo.member = &CommunityMembership{UserID: 1, TelegramUserID: 42, GroupChatID: -100, Status: "pending"}
	repo.invite = &CommunityInvite{ID: 2, UserID: 1, TelegramUserID: 42, GroupChatID: -100, BotID: 77, Status: "active", ExpiresAt: time.Now().Add(-time.Minute)}
	state, err := s.CreateInvite(context.Background(), 1, CommunityInviteInput{})
	require.NoError(t, err)
	require.Nil(t, state.Membership)
	require.NotNil(t, state.Invite)
	require.Zero(t, repo.invite.TelegramUserID)
	require.Equal(t, []string{"left"}, repo.marked)
}

func TestCommunityAdminListUsesConfiguredGroupAndNormalizesFilters(t *testing.T) {
	s, repo, telegram := communityTestService(t)
	page, err := s.ListMembers(context.Background(), CommunityMemberFilter{Page: 0, PageSize: 500, Search: "  user@example.com  ", Status: "NOT_JOINED"})
	require.NoError(t, err)
	require.EqualValues(t, -100, repo.listGroupID)
	require.EqualValues(t, 77, repo.listBotID)
	require.Equal(t, CommunityMemberFilter{Page: 1, PageSize: 100, Search: "user@example.com", Status: "not_joined"}, repo.listFilter)
	require.EqualValues(t, 3, page.Summary.Total)
	require.Empty(t, telegram.methods)
	_, err = s.ListMembers(context.Background(), CommunityMemberFilter{Status: "invalid"})
	require.ErrorIs(t, err, ErrCommunityInvalid)
}
