package main

import (
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const systemAdminRole = "system_admin"

// isSystemAdmin reports whether the user holds the system admin role.
func isSystemAdmin(user *model.User) bool {
	if user == nil {
		return false
	}
	for _, role := range strings.Fields(user.Roles) {
		if role == systemAdminRole {
			return true
		}
	}
	return false
}

// MessageWillBePosted blocks human-to-human direct and group messages.
// A non-empty returned string rejects the post and is shown to the user.
func (p *Plugin) MessageWillBePosted(c *plugin.Context, post *model.Post) (*model.Post, string) {
	cfg := p.getConfiguration()

	// 1. Look up the channel. If the type is unknown, fail open: blocking here
	//    would block posting in every channel during a transient error.
	channel, appErr := p.API.GetChannel(post.ChannelId)
	if appErr != nil {
		p.API.LogWarn("disallow-dm: get channel failed, allowing post", "channel_id", post.ChannelId, "error", appErr.Error())
		return post, ""
	}

	// 2. Only direct and group messages are ever blocked.
	if channel.Type != model.ChannelTypeDirect && channel.Type != model.ChannelTypeGroup {
		return post, ""
	}

	// 3. Group messages are blocked only when enabled.
	if channel.Type == model.ChannelTypeGroup && !cfg.BlockGroupMessages {
		return post, ""
	}

	// 4. Pure exceptions (no API call).
	if strings.HasPrefix(post.Type, "system_") {
		return post, ""
	}
	// An empty other-user id denotes a self-DM (channel name is id__id, system-generated),
	// so this check does not over-match real two-person DMs whose other-user id is always non-empty.
	if cfg.AllowSelfMessages && channel.Type == model.ChannelTypeDirect &&
		channel.GetOtherUserIdForDM(post.UserId) == "" {
		return post, ""
	}

	// From here the channel is a confirmed DM/GM: lookup errors fail closed.

	// 5. Author exceptions.
	author, appErr := p.API.GetUser(post.UserId)
	if appErr != nil {
		p.API.LogWarn("disallow-dm: get author failed, blocking post", "user_id", post.UserId, "error", appErr.Error())
		return nil, cfg.rejectionMessageOrDefault()
	}
	if author.IsBot {
		return post, ""
	}
	if cfg.AllowAdmins && isSystemAdmin(author) {
		return post, ""
	}

	// 6. Receiving-side exceptions: a bot anywhere, or (when enabled) an admin.
	participants, appErr := p.otherParticipants(channel, post.UserId)
	if appErr != nil {
		p.API.LogWarn("disallow-dm: get participants failed, blocking post", "channel_id", channel.Id, "error", appErr.Error())
		return nil, cfg.rejectionMessageOrDefault()
	}
	for _, u := range participants {
		if u.IsBot {
			return post, ""
		}
		if cfg.AllowAdmins && isSystemAdmin(u) {
			return post, ""
		}
	}

	// 7. Human-to-human DM/GM: reject.
	return nil, cfg.rejectionMessageOrDefault()
}

// otherParticipants returns the users on the other side of a DM/GM, excluding
// the author. For a self-DM it returns no users.
func (p *Plugin) otherParticipants(channel *model.Channel, authorID string) ([]*model.User, *model.AppError) {
	if channel.Type == model.ChannelTypeDirect {
		otherID := channel.GetOtherUserIdForDM(authorID)
		if otherID == "" {
			return nil, nil
		}
		user, appErr := p.API.GetUser(otherID)
		if appErr != nil {
			return nil, appErr
		}
		return []*model.User{user}, nil
	}

	members, appErr := p.API.GetUsersInChannel(channel.Id, model.ChannelSortByUsername, 0, 100)
	if appErr != nil {
		return nil, appErr
	}
	others := make([]*model.User, 0, len(members))
	for _, u := range members {
		if u.Id != authorID {
			others = append(others, u)
		}
	}
	return others, nil
}
