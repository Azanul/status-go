package communities

import (
	"crypto/ecdsa"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/stretchr/testify/suite"

	"github.com/status-im/status-go/eth-node/crypto"
	"github.com/status-im/status-go/protocol/common"
	"github.com/status-im/status-go/protocol/protobuf"
)

func TestCommunitySuite(t *testing.T) {
	suite.Run(t, new(CommunitySuite))
}

const testChatID1 = "chat-id-1"
const testCategoryID1 = "category-id-1"
const testCategoryName1 = "category-name-1"
const testChatID2 = "chat-id-2"

type TimeSourceStub struct {
}

func (t *TimeSourceStub) GetCurrentTime() uint64 {
	return uint64(time.Now().Unix())
}

type CommunitySuite struct {
	suite.Suite

	identity    *ecdsa.PrivateKey
	communityID []byte

	member1 *ecdsa.PrivateKey
	member2 *ecdsa.PrivateKey
	member3 *ecdsa.PrivateKey

	member1Key string
	member2Key string
	member3Key string
}

func (s *CommunitySuite) SetupTest() {
	identity, err := crypto.GenerateKey()
	s.Require().NoError(err)
	s.identity = identity
	s.communityID = crypto.CompressPubkey(&identity.PublicKey)

	member1, err := crypto.GenerateKey()
	s.Require().NoError(err)
	s.member1 = member1

	member2, err := crypto.GenerateKey()
	s.Require().NoError(err)
	s.member2 = member2

	member3, err := crypto.GenerateKey()
	s.Require().NoError(err)
	s.member3 = member3

	s.member1Key = common.PubkeyToHex(&s.member1.PublicKey)
	s.member2Key = common.PubkeyToHex(&s.member2.PublicKey)
	s.member3Key = common.PubkeyToHex(&s.member3.PublicKey)

}

func (s *CommunitySuite) TestHasPermission() {
	// returns false if empty public key is passed
	community := &Community{}
	ownerKey, err := crypto.GenerateKey()
	s.Require().NoError(err)

	nonMemberKey, err := crypto.GenerateKey()
	s.Require().NoError(err)

	memberKey, err := crypto.GenerateKey()
	s.Require().NoError(err)

	s.Require().False(community.hasRoles(nil, adminRole()))

	// returns false if key is passed, but config is nil
	s.Require().False(community.hasRoles(&nonMemberKey.PublicKey, adminRole()))

	// returns true if the user is the owner

	communityDescription := &protobuf.CommunityDescription{}
	communityDescription.Members = make(map[string]*protobuf.CommunityMember)
	communityDescription.Members[common.PubkeyToHex(&ownerKey.PublicKey)] = &protobuf.CommunityMember{Roles: []protobuf.CommunityMember_Roles{protobuf.CommunityMember_ROLE_OWNER}}
	communityDescription.Members[common.PubkeyToHex(&memberKey.PublicKey)] = &protobuf.CommunityMember{Roles: []protobuf.CommunityMember_Roles{protobuf.CommunityMember_ROLE_ADMIN}}

	community.config = &Config{ID: &ownerKey.PublicKey, CommunityDescription: communityDescription}

	s.Require().True(community.hasRoles(&ownerKey.PublicKey, ownerRole()))

	// return false if user is not a member
	s.Require().False(community.hasRoles(&nonMemberKey.PublicKey, adminRole()))

	// return true if user is a member and has permissions
	s.Require().True(community.hasRoles(&memberKey.PublicKey, adminRole()))

	// return false if user is a member and does not have permissions
	s.Require().False(community.hasRoles(&memberKey.PublicKey, ownerRole()))

}

func (s *CommunitySuite) TestCreateChat() {
	newChatID := "new-chat-id"
	org := s.buildCommunity(&s.identity.PublicKey)
	org.config.PrivateKey = nil
	org.config.ID = nil

	identity := &protobuf.ChatIdentity{
		DisplayName: "new-chat-display-name",
		Description: "new-chat-description",
	}
	permissions := &protobuf.CommunityPermissions{
		Access: protobuf.CommunityPermissions_AUTO_ACCEPT,
	}

	_, err := org.CreateChat(newChatID, &protobuf.CommunityChat{
		Identity:    identity,
		Permissions: permissions,
	})

	s.Require().Equal(ErrNotAuthorized, err)

	org.config.PrivateKey = s.identity
	org.config.ID = &s.identity.PublicKey

	changes, err := org.CreateChat(newChatID, &protobuf.CommunityChat{
		Identity:    identity,
		Permissions: permissions,
	})

	description := org.config.CommunityDescription

	s.Require().NoError(err)
	s.Require().NotNil(description)
	s.Require().NotNil(description.Chats[newChatID])
	s.Require().NotEmpty(description.Clock)
	s.Require().Equal(len(description.Chats)-1, int(description.Chats[newChatID].Position))
	s.Require().Equal(permissions, description.Chats[newChatID].Permissions)
	s.Require().Equal(identity, description.Chats[newChatID].Identity)

	s.Require().NotNil(changes)
	s.Require().NotNil(changes.ChatsAdded[newChatID])

	// Add a community with the same name

	_, err = org.CreateChat("different-chat-id", &protobuf.CommunityChat{
		Identity:    identity,
		Permissions: permissions,
	})

	s.Require().Error(err)
}

func (s *CommunitySuite) TestEditChat() {
	newChatID := "new-chat-id"
	org := s.buildCommunity(&s.identity.PublicKey)

	identity := &protobuf.ChatIdentity{
		DisplayName: "new-chat-display-name",
		Description: "new-chat-description",
		Emoji:       "😎",
		Color:       "#000000",
	}
	permissions := &protobuf.CommunityPermissions{
		Access:  protobuf.CommunityPermissions_AUTO_ACCEPT,
		Private: false,
	}

	_, err := org.CreateChat(newChatID, &protobuf.CommunityChat{
		Identity:                identity,
		Permissions:             permissions,
		HideIfPermissionsNotMet: false,
	})
	s.Require().NoError(err)

	org.config.PrivateKey = nil
	org.config.ID = nil
	editedIdentity := &protobuf.ChatIdentity{
		DisplayName: "edited-new-chat-display-name",
		Description: "edited-new-chat-description",
		Emoji:       "🤘",
		Color:       "#FFFFFF",
	}
	editedPermissions := &protobuf.CommunityPermissions{
		Access:  protobuf.CommunityPermissions_AUTO_ACCEPT,
		Private: true,
	}
	_, err = org.EditChat(newChatID, &protobuf.CommunityChat{
		Identity:    editedIdentity,
		Permissions: editedPermissions,
	})
	s.Require().Equal(ErrNotAuthorized, err)

	description := org.config.CommunityDescription
	org.config.PrivateKey = s.identity
	org.config.ID = &s.identity.PublicKey
	editChanges, err := org.EditChat(newChatID, &protobuf.CommunityChat{
		Identity:                editedIdentity,
		Permissions:             editedPermissions,
		HideIfPermissionsNotMet: true,
	})

	s.Require().NoError(err)

	s.Require().NotNil(description.Chats[newChatID])
	s.Require().NotEmpty(description.Clock)
	s.Require().Equal(editedPermissions, description.Chats[newChatID].Permissions)
	s.Require().Equal(editedIdentity, description.Chats[newChatID].Identity)

	s.Require().NotNil(editChanges)
	s.Require().NotNil(editChanges.ChatsModified[newChatID])
	s.Require().Equal(editChanges.ChatsModified[newChatID].ChatModified.Identity, editedIdentity)
	s.Require().Equal(editChanges.ChatsModified[newChatID].ChatModified.Permissions, editedPermissions)
	s.Require().Equal(editChanges.ChatsModified[newChatID].ChatModified.HideIfPermissionsNotMet, true)
}

func (s *CommunitySuite) TestDeleteChat() {
	org := s.buildCommunity(&s.identity.PublicKey)
	org.config.PrivateKey = nil
	org.config.ID = nil

	_, err := org.DeleteChat(testChatID1)
	s.Require().Equal(ErrNotAuthorized, err)
	change1Clock := org.Clock()

	org.config.PrivateKey = s.identity
	org.config.ID = &s.identity.PublicKey

	changes, err := org.DeleteChat(testChatID1)
	s.Require().NoError(err)
	s.Require().NotNil(changes)
	change2Clock := org.Clock()

	s.Require().Nil(org.Chats()[testChatID1])
	s.Require().Len(changes.ChatsRemoved, 1)
	s.Require().Greater(change2Clock, change1Clock)
}

func (s *CommunitySuite) TestRemoveUserFromChat() {
	org := s.buildCommunity(&s.identity.PublicKey)
	org.config.PrivateKey = nil
	org.config.ID = nil
	// Not an admin
	_, err := org.RemoveUserFromOrg(&s.member1.PublicKey)
	s.Require().Equal(ErrNotAuthorized, err)

	// Add admin to community
	org.config.PrivateKey = s.identity
	org.config.ID = &s.identity.PublicKey

	actualCommunity, err := org.RemoveUserFromChat(&s.member1.PublicKey, testChatID1)
	s.Require().Nil(err)
	s.Require().NotNil(actualCommunity)

	// Check member has not been removed
	s.Require().True(org.HasMember(&s.member1.PublicKey))

	// Check member has not been removed from org
	_, ok := actualCommunity.Members[common.PubkeyToHex(&s.member1.PublicKey)]
	s.Require().True(ok)

	// Check member has been removed from chat
	_, ok = actualCommunity.Chats[testChatID1].Members[common.PubkeyToHex(&s.member1.PublicKey)]
	s.Require().False(ok)
}

func (s *CommunitySuite) TestRemoveUserFormOrg() {
	org := s.buildCommunity(&s.identity.PublicKey)
	org.config.PrivateKey = nil
	org.config.ID = nil
	// Not an admin
	_, err := org.RemoveUserFromOrg(&s.member1.PublicKey)
	s.Require().Equal(ErrNotAuthorized, err)

	// Add admin to community
	org.config.PrivateKey = s.identity
	org.config.ID = &s.identity.PublicKey

	actualCommunity, err := org.RemoveUserFromOrg(&s.member1.PublicKey)
	s.Require().Nil(err)
	s.Require().NotNil(actualCommunity)

	// Check member has been removed
	s.Require().False(org.HasMember(&s.member1.PublicKey))

	// Check member has been removed from org
	_, ok := actualCommunity.Members[common.PubkeyToHex(&s.member1.PublicKey)]
	s.Require().False(ok)

	// Check member has been removed from chat
	_, ok = actualCommunity.Chats[testChatID1].Members[common.PubkeyToHex(&s.member1.PublicKey)]
	s.Require().False(ok)
}

func (s *CommunitySuite) TestRemoveOurselvesFormOrg() {
	org := s.buildCommunity(&s.identity.PublicKey)

	// We don't need to be an admin to remove ourselves from community
	org.config.PrivateKey = nil

	org.RemoveOurselvesFromOrg(&s.member1.PublicKey)

	// Check member has been removed from org
	s.Require().False(org.HasMember(&s.member1.PublicKey))

	// Check member has been removed from chat
	_, ok := org.config.CommunityDescription.Chats[testChatID1].Members[common.PubkeyToHex(&s.member1.PublicKey)]
	s.Require().False(ok)
}

func (s *CommunitySuite) TestAcceptRequestToJoin() {
	// WHAT TO DO WITH ENS
	// TEST CASE 1: Not an admin
	// TEST CASE 2: No request to join
	// TEST CASE 3: Valid
}

func (s *CommunitySuite) TestDeclineRequestToJoin() {
	// TEST CASE 1: Not an admin
	// TEST CASE 2: No request to join
	// TEST CASE 3: Valid
}

func (s *CommunitySuite) TestValidateRequestToJoin() {
	description := &protobuf.CommunityDescription{}

	key, err := crypto.GenerateKey()
	s.Require().NoError(err)

	signer := &key.PublicKey

	revealedAccounts := []*protobuf.RevealedAccount{
		&protobuf.RevealedAccount{
			Address: "0x0100000000000000000000000000000000000000"},
	}

	request := &protobuf.CommunityRequestToJoin{
		EnsName:          "donvanvliet.stateofus.eth",
		CommunityId:      s.communityID,
		Clock:            uint64(time.Now().Unix()),
		RevealedAccounts: revealedAccounts,
	}

	requestWithChatID := &protobuf.CommunityRequestToJoin{
		EnsName:          "donvanvliet.stateofus.eth",
		CommunityId:      s.communityID,
		ChatId:           testChatID1,
		Clock:            uint64(time.Now().Unix()),
		RevealedAccounts: revealedAccounts,
	}

	requestWithoutENS := &protobuf.CommunityRequestToJoin{
		CommunityId:      s.communityID,
		Clock:            uint64(time.Now().Unix()),
		RevealedAccounts: revealedAccounts,
	}

	requestWithChatWithoutENS := &protobuf.CommunityRequestToJoin{
		CommunityId:      s.communityID,
		ChatId:           testChatID1,
		Clock:            uint64(time.Now().Unix()),
		RevealedAccounts: revealedAccounts,
	}

	// MATRIX
	// NO_MEMBERHSIP - NO_MEMBERSHIP -> Error -> Anyone can join org, chat is read/write for anyone
	// NO_MEMBRISHIP - INVITATION_ONLY -> Error -> Anyone can join org, chat is invitation only
	// NO_MEMBERSHIP - ON_REQUEST -> Success -> Anyone can join org, chat is on request and needs approval
	// INVITATION_ONLY - NO_MEMBERSHIP -> TODO -> Org is invitation only, chat is read-write for members
	// INVITATION_ONLY - INVITATION_ONLY -> Error -> Org is invitation only, chat is invitation only
	// INVITATION_ONLY - ON_REQUEST -> TODO -> Error -> Org is invitation only, member of the org need to request access for chat
	// ON_REQUEST - NO_MEMBRERSHIP -> TODO -> Error -> Org is on request, chat is read write for members
	// ON_REQUEST - INVITATION_ONLY -> Error -> Org is on request, chat is invitation only for members
	// ON_REQUEST - ON_REQUEST -> Fine -> Org is on request, chat is on request

	testCases := []struct {
		name    string
		config  Config
		request *protobuf.CommunityRequestToJoin
		signer  *ecdsa.PublicKey
		err     error
	}{
		{
			name:    "on-request access to community",
			config:  s.configOnRequest(),
			signer:  signer,
			request: request,
			err:     nil,
		},
		{
			name:    "not admin",
			config:  Config{MemberIdentity: key, CommunityDescription: description},
			signer:  signer,
			request: request,
			err:     ErrNotAdmin,
		},
		{
			name:    "ens-only org and missing ens",
			config:  s.configENSOnly(),
			signer:  signer,
			request: requestWithoutENS,
			err:     ErrCantRequestAccess,
		},
		{
			name:    "ens-only chat and missing ens",
			config:  s.configChatENSOnly(),
			signer:  signer,
			request: requestWithChatWithoutENS,
			err:     ErrCantRequestAccess,
		},
		{
			name:    "missing chat",
			config:  s.configOnRequest(),
			signer:  signer,
			request: requestWithChatID,
			err:     ErrChatNotFound,
		},
		// Org-Chat combinations
		// NO_MEMBERSHIP-NO_MEMBERSHIP = error as you should not be
		// requesting access
		{
			name:    "no-membership org with no-membeship chat",
			config:  s.configNoMembershipOrgNoMembershipChat(),
			signer:  signer,
			request: requestWithChatID,
			err:     ErrCantRequestAccess,
		},
		// NO_MEMBERSHIP-ON_REQUEST = this is a valid case
		{
			name:    "no-membership org with on-request chat",
			config:  s.configNoMembershipOrgOnRequestChat(),
			signer:  signer,
			request: requestWithChatID,
		},
		// ON_REQUEST-ON_REQUEST success
		{
			name:    "on-request org with on-request chat",
			config:  s.configOnRequestOrgOnRequestChat(),
			signer:  signer,
			request: requestWithChatID,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			org, err := New(tc.config, &TimeSourceStub{}, &DescriptionEncryptorMock{}, nil)
			s.Require().NoError(err)
			err = org.ValidateRequestToJoin(tc.signer, tc.request)
			s.Require().Equal(tc.err, err)
		})
	}
}

func (s *CommunitySuite) TestCanPostCanView() {
	chatID := "chat-id"
	memberKey := common.PubkeyToHex(&s.member1.PublicKey)
	// Member has no channel role
	description := &protobuf.CommunityDescription{
		Members: map[string]*protobuf.CommunityMember{
			memberKey: &protobuf.CommunityMember{},
		},
		Chats: map[string]*protobuf.CommunityChat{
			chatID: &protobuf.CommunityChat{
				Members: map[string]*protobuf.CommunityMember{
					memberKey: &protobuf.CommunityMember{},
				},
			},
		},
	}

	community := &Community{config: &Config{ID: &s.member2.PublicKey}}
	community.config.CommunityDescription = description

	result, err := community.CanPost(&s.member1.PublicKey, chatID, protobuf.ApplicationMetadataMessage_CHAT_MESSAGE)
	s.Require().NoError(err)
	s.Require().True(result)

	result = community.CanView(&s.member1.PublicKey, chatID)
	s.Require().True(result)

	// member has view channel permissions
	description.Chats[chatID].Members[memberKey].ChannelRole = protobuf.CommunityMember_CHANNEL_ROLE_VIEWER

	result, err = community.CanPost(&s.member1.PublicKey, chatID, protobuf.ApplicationMetadataMessage_CHAT_MESSAGE)
	s.Require().NoError(err)
	s.Require().False(result)

	result = community.CanView(&s.member1.PublicKey, chatID)
	s.Require().True(result)
}

func (s *CommunitySuite) TestCanPost() {
	notMember := &s.member3.PublicKey
	member := &s.member1.PublicKey

	testCases := []struct {
		name    string
		config  Config
		member  *ecdsa.PublicKey
		err     error
		canPost bool
	}{
		{
			name:    "no-membership org with no-membership chat",
			config:  s.configNoMembershipOrgNoMembershipChat(),
			member:  notMember,
			canPost: false,
		},
		{
			name:    "membership org with no-membership chat-not-a-member",
			config:  s.configOnRequestOrgNoMembershipChat(),
			member:  notMember,
			canPost: false,
		},
		{
			name:    "membership org with no-membership chat",
			config:  s.configOnRequestOrgNoMembershipChat(),
			member:  member,
			canPost: true,
		},
		{
			name:    "creator can always post of course",
			config:  s.configOnRequestOrgNoMembershipChat(),
			member:  &s.identity.PublicKey,
			canPost: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			var err error
			org, err := New(tc.config, &TimeSourceStub{}, &DescriptionEncryptorMock{}, nil)
			s.Require().NoError(err)

			canPost, err := org.CanPost(tc.member, testChatID1, protobuf.ApplicationMetadataMessage_CHAT_MESSAGE)
			s.Require().Equal(tc.err, err)
			s.Require().Equal(tc.canPost, canPost)
		})
	}
}

func (s *CommunitySuite) TestHandleCommunityDescription() {
	key, err := crypto.GenerateKey()
	s.Require().NoError(err)

	signer := &key.PublicKey

	buildChanges := func(c *Community) *CommunityChanges {
		return c.emptyCommunityChanges()
	}

	testCases := []struct {
		name        string
		description func(*Community) *protobuf.CommunityDescription
		changes     func(*Community) *CommunityChanges
		signer      *ecdsa.PublicKey
		err         error
	}{
		{
			name:        "updated version but no changes",
			description: s.identicalCommunityDescription,
			signer:      signer,
			changes:     buildChanges,
			err:         nil,
		},
		{
			name:        "updated version but lower clock",
			description: s.oldCommunityDescription,
			signer:      signer,
			changes:     func(c *Community) *CommunityChanges { return nil },
			err:         ErrInvalidCommunityDescriptionClockOutdated,
		},
		{
			name:        "removed member from org",
			description: s.removedMemberCommunityDescription,
			signer:      signer,
			changes: func(org *Community) *CommunityChanges {
				changes := org.emptyCommunityChanges()
				changes.MembersRemoved[s.member1Key] = &protobuf.CommunityMember{}
				changes.ChatsModified[testChatID1] = &CommunityChatChanges{
					MembersAdded:   make(map[string]*protobuf.CommunityMember),
					MembersRemoved: make(map[string]*protobuf.CommunityMember),
				}
				changes.ChatsModified[testChatID1].MembersRemoved[s.member1Key] = &protobuf.CommunityMember{}

				return changes
			},
			err: nil,
		},
		{
			name:        "added member from org",
			description: s.addedMemberCommunityDescription,
			signer:      signer,
			changes: func(org *Community) *CommunityChanges {
				changes := org.emptyCommunityChanges()
				changes.MembersAdded[s.member3Key] = &protobuf.CommunityMember{}
				changes.ChatsModified[testChatID1] = &CommunityChatChanges{
					MembersAdded:   make(map[string]*protobuf.CommunityMember),
					MembersRemoved: make(map[string]*protobuf.CommunityMember),
				}
				changes.ChatsModified[testChatID1].MembersAdded[s.member3Key] = &protobuf.CommunityMember{}

				return changes
			},
			err: nil,
		},
		{
			name:        "chat added to org",
			description: s.addedChatCommunityDescription,
			signer:      signer,
			changes: func(org *Community) *CommunityChanges {
				changes := org.emptyCommunityChanges()
				changes.MembersAdded[s.member3Key] = &protobuf.CommunityMember{}
				changes.ChatsAdded[testChatID2] = &protobuf.CommunityChat{
					Identity:    &protobuf.ChatIdentity{DisplayName: "added-chat", Description: "description"},
					Permissions: &protobuf.CommunityPermissions{Access: protobuf.CommunityPermissions_MANUAL_ACCEPT},
					Members:     make(map[string]*protobuf.CommunityMember)}
				changes.ChatsAdded[testChatID2].Members[s.member3Key] = &protobuf.CommunityMember{}

				return changes
			},
			err: nil,
		},
		{
			name:        "chat removed from the org",
			description: s.removedChatCommunityDescription,
			signer:      signer,
			changes: func(org *Community) *CommunityChanges {
				changes := org.emptyCommunityChanges()
				changes.ChatsRemoved[testChatID1] = org.config.CommunityDescription.Chats[testChatID1]

				return changes
			},
			err: nil,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			org := s.buildCommunity(signer)
			org.Join()
			expectedChanges := tc.changes(org)
			actualChanges, err := org.UpdateCommunityDescription(tc.description(org), []byte{0x01}, nil)
			s.Require().Equal(tc.err, err)
			s.Require().Equal(expectedChanges, actualChanges)
		})
	}
}

func (s *CommunitySuite) TestValidateCommunityDescription() {

	testCases := []struct {
		name        string
		description *protobuf.CommunityDescription
		err         error
	}{
		{
			name:        "valid",
			description: s.buildCommunityDescription(),
			err:         nil,
		},
		{
			name: "empty description",
			err:  ErrInvalidCommunityDescription,
		},
		{
			name:        "empty org permissions",
			description: s.emptyPermissionsCommunityDescription(),
			err:         ErrInvalidCommunityDescriptionNoOrgPermissions,
		},
		{
			name:        "empty chat permissions",
			description: s.emptyChatPermissionsCommunityDescription(),
			err:         ErrInvalidCommunityDescriptionNoChatPermissions,
		},
		{
			name:        "unknown org permissions",
			description: s.unknownOrgPermissionsCommunityDescription(),
			err:         ErrInvalidCommunityDescriptionUnknownOrgAccess,
		},
		{
			name:        "unknown chat permissions",
			description: s.unknownChatPermissionsCommunityDescription(),
			err:         ErrInvalidCommunityDescriptionUnknownChatAccess,
		},
		{
			name:        "member in chat but not in org",
			description: s.memberInChatNotInOrgCommunityDescription(),
			err:         ErrInvalidCommunityDescriptionMemberInChatButNotInOrg,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			err := ValidateCommunityDescription(tc.description)
			s.Require().Equal(tc.err, err)
		})
	}
}

func (s *CommunitySuite) TestChatIDs() {
	community := s.buildCommunity(&s.identity.PublicKey)
	chatIDs := community.ChatIDs()

	s.Require().Len(chatIDs, 1)
}

func (s *CommunitySuite) TestChannelTokenPermissionsByType() {
	org := s.buildCommunity(&s.identity.PublicKey)

	viewOnlyPermissions := []*protobuf.CommunityTokenPermission{
		&protobuf.CommunityTokenPermission{
			Id:            "some-id",
			Type:          protobuf.CommunityTokenPermission_CAN_VIEW_CHANNEL,
			TokenCriteria: make([]*protobuf.TokenCriteria, 0),
			ChatIds:       []string{"some-chat-id"},
		},
	}

	viewAndPostPermissions := []*protobuf.CommunityTokenPermission{
		&protobuf.CommunityTokenPermission{
			Id:            "some-other-id",
			Type:          protobuf.CommunityTokenPermission_CAN_VIEW_AND_POST_CHANNEL,
			TokenCriteria: make([]*protobuf.TokenCriteria, 0),
			ChatIds:       []string{"some-chat-id-2"},
		},
	}

	for _, viewOnlyPermission := range viewOnlyPermissions {
		_, err := org.UpsertTokenPermission(viewOnlyPermission)
		s.Require().NoError(err)
	}
	for _, viewAndPostPermission := range viewAndPostPermissions {
		_, err := org.UpsertTokenPermission(viewAndPostPermission)
		s.Require().NoError(err)
	}

	result := org.ChannelTokenPermissionsByType("some-chat-id", protobuf.CommunityTokenPermission_CAN_VIEW_CHANNEL)
	s.Require().Len(result, 1)
	s.Require().Equal(result[0].Id, viewOnlyPermissions[0].Id)
	s.Require().Equal(result[0].TokenCriteria, viewOnlyPermissions[0].TokenCriteria)
	s.Require().Equal(result[0].ChatIds, viewOnlyPermissions[0].ChatIds)

	result = org.ChannelTokenPermissionsByType("some-chat-id-2", protobuf.CommunityTokenPermission_CAN_VIEW_AND_POST_CHANNEL)
	s.Require().Len(result, 1)
	s.Require().Equal(result[0].Id, viewAndPostPermissions[0].Id)
	s.Require().Equal(result[0].TokenCriteria, viewAndPostPermissions[0].TokenCriteria)
	s.Require().Equal(result[0].ChatIds, viewAndPostPermissions[0].ChatIds)
}

func (s *CommunitySuite) TestChannelEncrypted() {
	org := s.buildCommunity(&s.identity.PublicKey)
	someChannelID := "some-channel-id"
	someChatID := org.ChatID(someChannelID)

	s.Require().False(org.ChannelEncrypted(someChannelID))

	_, err := org.UpsertTokenPermission(&protobuf.CommunityTokenPermission{
		Id:            "A",
		Type:          protobuf.CommunityTokenPermission_CAN_VIEW_AND_POST_CHANNEL,
		TokenCriteria: []*protobuf.TokenCriteria{},
		ChatIds:       []string{someChatID},
	})
	s.Require().NoError(err)
	s.Require().True(org.channelEncrypted(someChannelID))

	_, err = org.UpsertTokenPermission(&protobuf.CommunityTokenPermission{
		Id:            "B",
		Type:          protobuf.CommunityTokenPermission_CAN_VIEW_CHANNEL,
		TokenCriteria: []*protobuf.TokenCriteria{&protobuf.TokenCriteria{}},
		ChatIds:       []string{someChatID},
	})
	s.Require().NoError(err)
	s.Require().True(org.channelEncrypted(someChannelID))

	// Channels with `view` permission without token requirements shouldn't be encrypted.
	// See: https://github.com/status-im/status-desktop/issues/14748
	_, err = org.UpsertTokenPermission(&protobuf.CommunityTokenPermission{
		Id:            "C",
		Type:          protobuf.CommunityTokenPermission_CAN_VIEW_CHANNEL,
		TokenCriteria: []*protobuf.TokenCriteria{},
		ChatIds:       []string{someChatID},
	})
	s.Require().NoError(err)
	s.Require().False(org.channelEncrypted(someChannelID))
}

func (s *CommunitySuite) emptyCommunityDescription() *protobuf.CommunityDescription {
	return &protobuf.CommunityDescription{
		Permissions: &protobuf.CommunityPermissions{},
	}

}

func (s *CommunitySuite) emptyCommunityDescriptionWithChat() *protobuf.CommunityDescription {
	desc := &protobuf.CommunityDescription{
		Members:     make(map[string]*protobuf.CommunityMember),
		Clock:       1,
		Chats:       make(map[string]*protobuf.CommunityChat),
		Categories:  make(map[string]*protobuf.CommunityCategory),
		Permissions: &protobuf.CommunityPermissions{},
	}

	desc.Categories[testCategoryID1] = &protobuf.CommunityCategory{CategoryId: testCategoryID1, Name: testCategoryName1, Position: 0}
	desc.Chats[testChatID1] = &protobuf.CommunityChat{Position: 0, Permissions: &protobuf.CommunityPermissions{}, Members: make(map[string]*protobuf.CommunityMember)}
	desc.Members[common.PubkeyToHex(&s.member1.PublicKey)] = &protobuf.CommunityMember{}
	desc.Chats[testChatID1].Members[common.PubkeyToHex(&s.member1.PublicKey)] = &protobuf.CommunityMember{}

	return desc

}

func (s *CommunitySuite) newConfig(identity *ecdsa.PrivateKey, description *protobuf.CommunityDescription) Config {
	return Config{
		MemberIdentity:       identity,
		ID:                   &identity.PublicKey,
		CommunityDescription: description,
		PrivateKey:           identity,
		ControlNode:          &identity.PublicKey,
		ControlDevice:        true,
	}
}

func (s *CommunitySuite) configOnRequest() Config {
	description := s.emptyCommunityDescription()
	description.Permissions.Access = protobuf.CommunityPermissions_MANUAL_ACCEPT
	return s.newConfig(s.identity, description)
}

func (s *CommunitySuite) configNoMembershipOrgNoMembershipChat() Config {
	description := s.emptyCommunityDescriptionWithChat()
	description.Permissions.Access = protobuf.CommunityPermissions_AUTO_ACCEPT
	description.Chats[testChatID1].Permissions.Access = protobuf.CommunityPermissions_AUTO_ACCEPT
	return s.newConfig(s.identity, description)
}

func (s *CommunitySuite) configNoMembershipOrgOnRequestChat() Config {
	description := s.emptyCommunityDescriptionWithChat()
	description.Permissions.Access = protobuf.CommunityPermissions_AUTO_ACCEPT
	description.Chats[testChatID1].Permissions.Access = protobuf.CommunityPermissions_MANUAL_ACCEPT
	return s.newConfig(s.identity, description)
}

func (s *CommunitySuite) configOnRequestOrgOnRequestChat() Config {
	description := s.emptyCommunityDescriptionWithChat()
	description.Permissions.Access = protobuf.CommunityPermissions_MANUAL_ACCEPT
	description.Chats[testChatID1].Permissions.Access = protobuf.CommunityPermissions_MANUAL_ACCEPT
	return s.newConfig(s.identity, description)
}

func (s *CommunitySuite) configOnRequestOrgNoMembershipChat() Config {
	description := s.emptyCommunityDescriptionWithChat()
	description.Permissions.Access = protobuf.CommunityPermissions_MANUAL_ACCEPT
	description.Chats[testChatID1].Permissions.Access = protobuf.CommunityPermissions_AUTO_ACCEPT
	return s.newConfig(s.identity, description)
}

func (s *CommunitySuite) configChatENSOnly() Config {
	description := s.emptyCommunityDescriptionWithChat()
	description.Permissions.Access = protobuf.CommunityPermissions_MANUAL_ACCEPT
	description.Chats[testChatID1].Permissions.Access = protobuf.CommunityPermissions_MANUAL_ACCEPT
	description.Chats[testChatID1].Permissions.EnsOnly = true
	return s.newConfig(s.identity, description)
}

func (s *CommunitySuite) configENSOnly() Config {
	description := s.emptyCommunityDescription()
	description.Permissions.Access = protobuf.CommunityPermissions_MANUAL_ACCEPT
	description.Permissions.EnsOnly = true
	return s.newConfig(s.identity, description)
}

func (s *CommunitySuite) config() Config {
	config := s.configOnRequestOrgOnRequestChat()
	return config
}

func (s *CommunitySuite) buildCommunityDescription() *protobuf.CommunityDescription {
	config := s.configOnRequestOrgOnRequestChat()
	desc := config.CommunityDescription
	desc.Clock = 1
	desc.Members = make(map[string]*protobuf.CommunityMember)
	desc.Members[s.member1Key] = &protobuf.CommunityMember{}
	desc.Members[s.member2Key] = &protobuf.CommunityMember{}
	desc.Chats[testChatID1].Members = make(map[string]*protobuf.CommunityMember)
	desc.Chats[testChatID1].Members[s.member1Key] = &protobuf.CommunityMember{}
	desc.Chats[testChatID1].Identity = &protobuf.ChatIdentity{
		DisplayName: "display-name",
		Description: "description",
	}
	return desc
}

func (s *CommunitySuite) emptyPermissionsCommunityDescription() *protobuf.CommunityDescription {
	desc := s.buildCommunityDescription()
	desc.Permissions = nil
	return desc
}

func (s *CommunitySuite) emptyChatPermissionsCommunityDescription() *protobuf.CommunityDescription {
	desc := s.buildCommunityDescription()
	desc.Chats[testChatID1].Permissions = nil
	return desc
}

func (s *CommunitySuite) unknownOrgPermissionsCommunityDescription() *protobuf.CommunityDescription {
	desc := s.buildCommunityDescription()
	desc.Permissions.Access = protobuf.CommunityPermissions_UNKNOWN_ACCESS
	return desc
}

func (s *CommunitySuite) unknownChatPermissionsCommunityDescription() *protobuf.CommunityDescription {
	desc := s.buildCommunityDescription()
	desc.Chats[testChatID1].Permissions.Access = protobuf.CommunityPermissions_UNKNOWN_ACCESS
	return desc
}

func (s *CommunitySuite) memberInChatNotInOrgCommunityDescription() *protobuf.CommunityDescription {
	desc := s.buildCommunityDescription()
	desc.Chats[testChatID1].Members[s.member3Key] = &protobuf.CommunityMember{}
	return desc
}

func (s *CommunitySuite) buildCommunity(owner *ecdsa.PublicKey) *Community {
	config := s.config()
	config.ID = owner
	config.CommunityDescription = s.buildCommunityDescription()

	org, err := New(config, &TimeSourceStub{}, &DescriptionEncryptorMock{}, nil)
	s.Require().NoError(err)
	return org
}

func (s *CommunitySuite) identicalCommunityDescription(org *Community) *protobuf.CommunityDescription {
	description := proto.Clone(org.config.CommunityDescription).(*protobuf.CommunityDescription)
	description.Clock++
	return description
}

func (s *CommunitySuite) oldCommunityDescription(org *Community) *protobuf.CommunityDescription {
	description := proto.Clone(org.config.CommunityDescription).(*protobuf.CommunityDescription)
	description.Clock--
	delete(description.Members, s.member1Key)
	delete(description.Chats[testChatID1].Members, s.member1Key)
	return description
}

func (s *CommunitySuite) removedMemberCommunityDescription(org *Community) *protobuf.CommunityDescription {
	description := proto.Clone(org.config.CommunityDescription).(*protobuf.CommunityDescription)
	description.Clock++
	delete(description.Members, s.member1Key)
	delete(description.Chats[testChatID1].Members, s.member1Key)
	return description
}

func (s *CommunitySuite) addedMemberCommunityDescription(org *Community) *protobuf.CommunityDescription {
	description := proto.Clone(org.config.CommunityDescription).(*protobuf.CommunityDescription)
	description.Clock++
	description.Members[s.member3Key] = &protobuf.CommunityMember{}
	description.Chats[testChatID1].Members[s.member3Key] = &protobuf.CommunityMember{}

	return description
}

func (s *CommunitySuite) addedChatCommunityDescription(org *Community) *protobuf.CommunityDescription {
	description := proto.Clone(org.config.CommunityDescription).(*protobuf.CommunityDescription)
	description.Clock++
	description.Members[s.member3Key] = &protobuf.CommunityMember{}
	description.Chats[testChatID2] = &protobuf.CommunityChat{
		Identity:    &protobuf.ChatIdentity{DisplayName: "added-chat", Description: "description"},
		Permissions: &protobuf.CommunityPermissions{Access: protobuf.CommunityPermissions_MANUAL_ACCEPT},
		Members:     make(map[string]*protobuf.CommunityMember)}
	description.Chats[testChatID2].Members[s.member3Key] = &protobuf.CommunityMember{}

	return description
}

func (s *CommunitySuite) removedChatCommunityDescription(org *Community) *protobuf.CommunityDescription {
	description := proto.Clone(org.config.CommunityDescription).(*protobuf.CommunityDescription)
	description.Clock++
	delete(description.Chats, testChatID1)

	return description
}

func (s *CommunitySuite) TestMarshalJSON() {
	community := s.buildCommunity(&s.identity.PublicKey)
	channelID := community.ChatID(testChatID1)
	_, err := community.UpsertTokenPermission(&protobuf.CommunityTokenPermission{
		Id:            "A",
		Type:          protobuf.CommunityTokenPermission_CAN_VIEW_AND_POST_CHANNEL,
		TokenCriteria: []*protobuf.TokenCriteria{},
		ChatIds:       []string{channelID},
	})
	s.Require().NoError(err)

	s.Require().True(community.ChannelEncrypted(testChatID1))

	communityDescription := community.config.CommunityDescription
	ownerKey := s.identity
	s.Require().NoError(err)

	memberKey, err := crypto.GenerateKey()
	s.Require().NoError(err)

	// returns true if the user is the owner

	communityDescription.Members = make(map[string]*protobuf.CommunityMember)
	communityDescription.Members[common.PubkeyToHex(&ownerKey.PublicKey)] = &protobuf.CommunityMember{Roles: []protobuf.CommunityMember_Roles{protobuf.CommunityMember_ROLE_OWNER}}
	communityDescription.Members[common.PubkeyToHex(&memberKey.PublicKey)] = &protobuf.CommunityMember{Roles: []protobuf.CommunityMember_Roles{protobuf.CommunityMember_ROLE_ADMIN}}
	communityDescription.Chats[testChatID1] = &protobuf.CommunityChat{Members: make(map[string]*protobuf.CommunityMember), Identity: &protobuf.ChatIdentity{}}
	communityDescription.Chats[testChatID1].Members[common.PubkeyToHex(&ownerKey.PublicKey)] = &protobuf.CommunityMember{Roles: []protobuf.CommunityMember_Roles{protobuf.CommunityMember_ROLE_OWNER}}

	// Test token gated community
	s.Require().True(community.ChannelEncrypted(testChatID1))
	communityJSON, err := json.Marshal(community)
	s.Require().NoError(err)

	var communityData map[string]interface{}
	err = json.Unmarshal(communityJSON, &communityData)
	s.Require().NoError(err)
	s.Require().NotNil(communityData["chats"])

	expectedChats := map[string]interface{}{}
	expectedChat := map[string]interface{}{
		"canPost":                 true,
		"canPostReactions":        true,
		"categoryID":              "",
		"canView":                 true,
		"color":                   "",
		"description":             "",
		"emoji":                   "",
		"hideIfPermissionsNotMet": false,
		"members": map[string]interface{}{
			common.PubkeyToHex(&ownerKey.PublicKey): map[string]interface{}{
				"roles": []interface{}{float64(1)},
			},
		},
		"id":                      testChatID1,
		"name":                    "",
		"permissions":             nil,
		"position":                float64(0),
		"tokenGated":              true,
		"viewersCanPostReactions": false,
		"missingEncryptionKey":    false,
	}

	expectedChats[testChatID1] = expectedChat
	s.Require().Equal(expectedChats, communityData["chats"])

	// Test token gated community
	community.config.CommunityDescription.TokenPermissions = nil
	communityJSON, err = json.Marshal(community)
	s.Require().NoError(err)

	err = json.Unmarshal(communityJSON, &communityData)
	s.Require().NoError(err)
	s.Require().NotNil(communityData["chats"])

	expectedChats = map[string]interface{}{}
	expectedChat = map[string]interface{}{
		"canPost":                 true,
		"canPostReactions":        true,
		"categoryID":              "",
		"canView":                 true,
		"color":                   "",
		"description":             "",
		"emoji":                   "",
		"hideIfPermissionsNotMet": false,
		"id":                      testChatID1,
		"members":                 nil,
		"name":                    "",
		"permissions":             nil,
		"position":                float64(0),
		"tokenGated":              false,
		"viewersCanPostReactions": false,
		"missingEncryptionKey":    false,
	}

	expectedChats[testChatID1] = expectedChat
	s.Require().Equal(expectedChats, communityData["chats"])
}
