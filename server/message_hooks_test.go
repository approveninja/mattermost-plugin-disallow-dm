package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const (
	humanID  = "humanaaaaaaaaaaaaaaaaaaaaaa"
	human2ID = "humanbbbbbbbbbbbbbbbbbbbbbb"
	botID    = "botccccccccccccccccccccccccc"
	adminID  = "adminddddddddddddddddddddddd"
)

func human(id string) *model.User  { return &model.User{Id: id, Roles: "system_user"} }
func bot(id string) *model.User    { return &model.User{Id: id, Roles: "system_user", IsBot: true} }
func admin(id string) *model.User  { return &model.User{Id: id, Roles: "system_user system_admin"} }

func dmChannel(a, b string) *model.Channel {
	return &model.Channel{Id: "channel_dm", Type: model.ChannelTypeDirect, Name: model.GetDMNameFromIds(a, b)}
}
func gmChannel() *model.Channel {
	return &model.Channel{Id: "channel_gm", Type: model.ChannelTypeGroup, Name: "groupchannel"}
}
func openChannel() *model.Channel {
	return &model.Channel{Id: "channel_open", Type: model.ChannelTypeOpen, Name: "town-square"}
}

func defaultConfig() *configuration {
	return &configuration{BlockGroupMessages: true, AllowAdmins: true, AllowSelfMessages: true}
}

// registerUsers wires GetUser for all canonical users as optional expectations.
func registerUsers(api *plugintest.API) {
	api.On("GetUser", humanID).Return(human(humanID), nil).Maybe()
	api.On("GetUser", human2ID).Return(human(human2ID), nil).Maybe()
	api.On("GetUser", botID).Return(bot(botID), nil).Maybe()
	api.On("GetUser", adminID).Return(admin(adminID), nil).Maybe()
}

func newAPI() *plugintest.API {
	api := &plugintest.API{}
	api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	return api
}

func runHook(cfg *configuration, api *plugintest.API, post *model.Post) string {
	p := &Plugin{configuration: cfg}
	p.API = api
	_, msg := p.MessageWillBePosted(&plugin.Context{}, post)
	return msg
}

func TestMessageWillBePosted(t *testing.T) {
	cases := []struct {
		name         string
		cfg          *configuration
		setup        func(api *plugintest.API)
		post         *model.Post
		wantRejected bool
	}{
		{
			name: "DM human to human is rejected",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_dm").Return(dmChannel(humanID, human2ID), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm"},
			wantRejected: true,
		},
		{
			name: "GM of humans is rejected when blocking on",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_gm").Return(gmChannel(), nil)
				api.On("GetUsersInChannel", "channel_gm", mock.Anything, mock.Anything, mock.Anything).
					Return([]*model.User{human(humanID), human(human2ID)}, nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_gm"},
			wantRejected: true,
		},
		{
			name: "GM allowed when BlockGroupMessages is false",
			cfg:  &configuration{BlockGroupMessages: false, AllowAdmins: true, AllowSelfMessages: true},
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_gm").Return(gmChannel(), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_gm"},
			wantRejected: false,
		},
		{
			name: "public channel is allowed",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				api.On("GetChannel", "channel_open").Return(openChannel(), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_open"},
			wantRejected: false,
		},
		{
			name: "bot author in DM is allowed",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_dm").Return(dmChannel(botID, humanID), nil)
			},
			post:         &model.Post{UserId: botID, ChannelId: "channel_dm"},
			wantRejected: false,
		},
		{
			name: "bot recipient in DM is allowed",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_dm").Return(dmChannel(humanID, botID), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm"},
			wantRejected: false,
		},
		{
			name: "system post in DM is allowed",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				api.On("GetChannel", "channel_dm").Return(dmChannel(humanID, human2ID), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm", Type: "system_join_channel"},
			wantRejected: false,
		},
		{
			name: "admin author allowed when AllowAdmins on",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_dm").Return(dmChannel(adminID, humanID), nil)
			},
			post:         &model.Post{UserId: adminID, ChannelId: "channel_dm"},
			wantRejected: false,
		},
		{
			name: "admin recipient allowed when AllowAdmins on",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_dm").Return(dmChannel(humanID, adminID), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm"},
			wantRejected: false,
		},
		{
			name: "admin member in GM allowed when AllowAdmins on",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_gm").Return(gmChannel(), nil)
				api.On("GetUsersInChannel", "channel_gm", mock.Anything, mock.Anything, mock.Anything).
					Return([]*model.User{human(humanID), human(human2ID), admin(adminID)}, nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_gm"},
			wantRejected: false,
		},
		{
			name: "admin author rejected when AllowAdmins off",
			cfg:  &configuration{BlockGroupMessages: true, AllowAdmins: false, AllowSelfMessages: true},
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_dm").Return(dmChannel(adminID, humanID), nil)
			},
			post:         &model.Post{UserId: adminID, ChannelId: "channel_dm"},
			wantRejected: true,
		},
		{
			name: "admin recipient rejected when AllowAdmins off",
			cfg:  &configuration{BlockGroupMessages: true, AllowAdmins: false, AllowSelfMessages: true},
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_dm").Return(dmChannel(humanID, adminID), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm"},
			wantRejected: true,
		},
		{
			name: "self-DM allowed when AllowSelfMessages on",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				api.On("GetChannel", "channel_dm").Return(dmChannel(humanID, humanID), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm"},
			wantRejected: false,
		},
		{
			name: "self-DM rejected when AllowSelfMessages off",
			cfg:  &configuration{BlockGroupMessages: true, AllowAdmins: true, AllowSelfMessages: false},
			setup: func(api *plugintest.API) {
				registerUsers(api)
				api.On("GetChannel", "channel_dm").Return(dmChannel(humanID, humanID), nil)
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm"},
			wantRejected: true,
		},
		{
			name: "GetChannel error fails open (allowed)",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				api.On("GetChannel", "channel_dm").
					Return(nil, model.NewAppError("GetChannel", "boom", nil, "boom", 500))
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm"},
			wantRejected: false,
		},
		{
			name: "author lookup error in confirmed DM fails closed (rejected)",
			cfg:  defaultConfig(),
			setup: func(api *plugintest.API) {
				api.On("GetChannel", "channel_dm").Return(dmChannel(humanID, human2ID), nil)
				api.On("GetUser", humanID).
					Return(nil, model.NewAppError("GetUser", "boom", nil, "boom", 500))
			},
			post:         &model.Post{UserId: humanID, ChannelId: "channel_dm"},
			wantRejected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newAPI()
			tc.setup(api)
			msg := runHook(tc.cfg, api, tc.post)
			if tc.wantRejected {
				assert.NotEmpty(t, msg, "expected the post to be rejected")
			} else {
				assert.Empty(t, msg, "expected the post to be allowed")
			}
		})
	}
}

func TestIsSystemAdmin(t *testing.T) {
	assert.True(t, isSystemAdmin(&model.User{Roles: "system_user system_admin"}))
	assert.False(t, isSystemAdmin(&model.User{Roles: "system_user"}))
	assert.False(t, isSystemAdmin(nil))
}
