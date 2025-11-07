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
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
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

func (ic *IRCClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	netName, channel, err := parsePortalID(portal.ID)
	if err != nil {
		return nil, err
	} else if netName != ic.NetMeta.Name {
		return nil, fmt.Errorf("%w (netname mismatch %q != %q)", bridgev2.ErrResolveIdentifierTryNext, netName, ic.NetMeta.Name)
	}
	var info bridgev2.ChatInfo
	if ic.isDM(channel) {
		info = bridgev2.ChatInfo{
			Members: &bridgev2.ChatMemberList{
				IsFull:                     true,
				ExcludeChangesFromTimeline: true,
				TotalMemberCount:           2,
				OtherUserID:                ic.makeUserID(channel),
				MemberMap:                  bridgev2.ChatMemberMap{},
			},
			Type: ptr.Ptr(database.RoomTypeDM),
		}
		info.Members.MemberMap.Add(bridgev2.ChatMember{
			EventSender: ic.makeEventSender(ic.Conn.CurrentNick()),
		})
		info.Members.MemberMap.Add(bridgev2.ChatMember{
			EventSender: ic.makeEventSender(channel),
		})
	} else {
		realInfo := ic.getCachedChatInfo(channel)
		if realInfo == nil {
			return &bridgev2.ChatInfo{
				Name: ptr.Ptr(channel),
			}, nil
		}
		info = bridgev2.ChatInfo{
			Name:  ptr.Ptr(channel),
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
	}
	return &info, nil
}

func (ic *IRCClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	netName, nick, err := ic.parseUserID(ghost.ID)
	if err != nil {
		return nil, err
	} else if netName != ic.NetMeta.Name {
		return nil, fmt.Errorf("%w (netname mismatch %q != %q)", bridgev2.ErrResolveIdentifierTryNext, netName, ic.NetMeta.Name)
	}
	var secure string
	if ic.NetMeta.TLS {
		secure = "s"
	}
	return &bridgev2.UserInfo{
		Identifiers: []string{fmt.Sprintf("irc%s://%s/%s", secure, ic.NetMeta.Address, nick)},
		Name:        &nick,
	}, nil
}
