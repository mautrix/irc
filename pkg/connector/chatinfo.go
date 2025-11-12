// mautrix-irc - A Matrix-IRC puppeting bridge.
// Copyright (C) 2025 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package connector

import (
	"context"
	"fmt"
	"iter"
	"time"

	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
)

type ChatInfoCache struct {
	Topic           string
	TopicSetBy      networkid.UserID
	TopicSetAt      time.Time
	Members         map[string]int
	MembersComplete bool
}

func (ic *IRCClient) getCachedChatInfo(channel string) *ChatInfoCache {
	ic.chatInfoCacheLock.RLock()
	defer ic.chatInfoCacheLock.RUnlock()
	return ic.chatInfoCache[channel]
}

type memberTuple struct {
	Name       string
	Meta       *ChatInfoCache
	PowerLevel int
}

type channelTuple memberTuple

func (ic *IRCClient) unlockedFindChannelsOfMember(nick string) iter.Seq[channelTuple] {
	return func(yield func(channelTuple) bool) {
		for channel, info := range ic.chatInfoCache {
			if pl, ok := info.Members[nick]; ok {
				yield(channelTuple{Name: channel, Meta: info, PowerLevel: pl})
			}
		}
	}
}

func (ic *IRCClient) unlockedGetOrCreateChatInfo(channel string) *ChatInfoCache {
	info, ok := ic.chatInfoCache[channel]
	if !ok {
		info = &ChatInfoCache{
			Members: make(map[string]int),
		}
		ic.chatInfoCache[channel] = info
	}
	return info
}

func (ic *IRCClient) getDMInfo(nick string) *bridgev2.ChatInfo {
	nick = ic.isupport.CaseMapping(nick)
	realNickName := ic.casemappedNames.GetDefault(nick, nick)
	info := &bridgev2.ChatInfo{
		Members: &bridgev2.ChatMemberList{
			IsFull:                     true,
			ExcludeChangesFromTimeline: true,
			TotalMemberCount:           2,
			OtherUserID:                ic.makeUserID(realNickName),
			MemberMap:                  bridgev2.ChatMemberMap{},
		},
		Type: ptr.Ptr(database.RoomTypeDM),
	}
	info.Members.MemberMap.Add(bridgev2.ChatMember{
		EventSender: ic.makeEventSender(ic.Conn.CurrentNick()),
	})
	info.Members.MemberMap.Add(bridgev2.ChatMember{
		EventSender: ic.makeEventSender(realNickName),
	})
	return info
}

func (ic *IRCClient) getChannelInfo(channel string) *bridgev2.ChatInfo {
	channel = ic.isupport.CaseMapping(channel)
	realChannelName := ic.casemappedNames.GetDefault(channel, channel)
	realInfo := ic.getCachedChatInfo(channel)
	if realInfo == nil {
		return &bridgev2.ChatInfo{
			Name: ptr.Ptr(realChannelName),
		}
	}
	info := &bridgev2.ChatInfo{
		Name:  ptr.Ptr(realChannelName),
		Topic: ptr.Ptr(realInfo.Topic),
		Members: &bridgev2.ChatMemberList{
			IsFull:                     realInfo.MembersComplete,
			CheckAllLogins:             false,
			ExcludeChangesFromTimeline: true,
			TotalMemberCount:           len(realInfo.Members),
			MemberMap:                  bridgev2.ChatMemberMap{},
		},
	}
	for nick, pl := range realInfo.Members {
		info.Members.MemberMap.Add(bridgev2.ChatMember{
			EventSender: ic.makeEventSender(nick),
			PowerLevel:  &pl,
		})
	}
	return info
}

func (ic *IRCClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	channel, err := ic.parsePortalID(portal.ID)
	if err != nil {
		return nil, err
	}
	if ic.isDM(channel) {
		return ic.getDMInfo(channel), nil
	} else {
		return ic.getChannelInfo(channel), nil
	}
}

func (ic *IRCClient) getUserInfo(nick string) *bridgev2.UserInfo {
	var secure string
	if ic.NetMeta.TLS {
		secure = "s"
	}
	nick = ic.isupport.CaseMapping(nick)
	realNick := ic.casemappedNames.GetDefault(nick, nick)
	return &bridgev2.UserInfo{
		Identifiers: []string{fmt.Sprintf("irc%s://%s/%s", secure, ic.NetMeta.Address, nick)},
		Name:        &realNick,
	}
}

func (ic *IRCClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	nick, err := ic.parseUserID(ghost.ID)
	if err != nil {
		return nil, err
	}
	return ic.getUserInfo(nick), nil
}

var _ bridgev2.PortalBridgeInfoFillingNetwork = (*IRCConnector)(nil)
var _ bridgev2.PersonalFilteringCustomizingNetworkAPI = (*IRCClient)(nil)

func (ic *IRCConnector) FillPortalBridgeInfo(portal *bridgev2.Portal, content *event.BridgeEventContent) {
	netName, channel, err := parsePortalID(portal.ID)
	if err != nil {
		return
	}
	netMeta, ok := ic.Config.Networks[netName]
	if !ok {
		return
	}
	var secure string
	if netMeta.TLS {
		secure = "s"
	}
	content.Channel.ExternalURL = fmt.Sprintf("irc%s://%s/%s", secure, netMeta.Address, channel)
	content.Network = &event.BridgeInfoSection{
		ID:          netName,
		DisplayName: netMeta.DisplayName,
		AvatarURL:   netMeta.AvatarURL,
		ExternalURL: netMeta.ExternalURL,
	}
}

func (ic *IRCClient) CustomizePersonalFilteringSpace(req *mautrix.ReqCreateRoom) {
	req.Name = ic.NetMeta.DisplayName
	req.Topic = fmt.Sprintf("Your %s bridged chats", req.Name)
	for _, evt := range req.InitialState {
		if evt.Type == event.StateRoomAvatar && ic.NetMeta.AvatarURL != "" {
			evt.Content.Parsed.(*event.RoomAvatarEventContent).URL = ic.NetMeta.AvatarURL
		}
	}
}
