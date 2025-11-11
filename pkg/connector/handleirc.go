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
	"strconv"
	"strings"
	"time"

	"github.com/ergochat/irc-go/ircmsg"
	"github.com/rs/zerolog"
	"go.mau.fi/util/ptr"
	"go.mau.fi/util/variationselector"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-irc/pkg/ircfmt"
)

func (ic *IRCClient) onConnect(msg ircmsg.Message) {
	ic.UserLogin.RemoteName = fmt.Sprintf("%s on %s", ic.Conn.CurrentNick(), ic.NetMeta.DisplayName)
	ic.UserLogin.RemoteProfile.Name = ic.Conn.CurrentNick()
	ic.UserLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnected})
	ic.isupport = ParseISupport(ic.Conn.ISupport())
	ic.UserLogin.Log.Trace().Any("evt", msg).Msg("Connected to network")
	for _, ch := range ic.UserLogin.Metadata.(*UserLoginMetadata).Channels {
		err := ic.Conn.Join(ch)
		if err != nil {
			ic.UserLogin.Log.Err(err).Str("channel_name", ch).
				Msg("Failed to auto-join channel")
			break
		}
	}
}

func (ic *IRCClient) onNick(msg ircmsg.Message) {
	ic.chatInfoCacheLock.Lock()
	defer ic.chatInfoCacheLock.Unlock()
	prevNick := msg.Nick()
	newNick := msg.Params[0]
	if prevNick == "" || newNick == "" {
		return
	}
	// TODO transfer DM portals? (need to be careful about security)
	for ch := range ic.unlockedFindChannelsOfMember(prevNick) {
		delete(ch.Meta.Members, prevNick)
		ch.Meta.Members[newNick] = ch.PowerLevel
		mm := bridgev2.ChatMemberMap{}
		mm.Set(bridgev2.ChatMember{
			EventSender: ic.makeEventSender(prevNick),
			Membership:  event.MembershipLeave,
			PowerLevel:  ptr.Ptr(0),
			MemberEventExtra: map[string]any{
				"reason": fmt.Sprintf("Changed nick to %s", newNick),
			},
			PrevMembership: event.MembershipJoin,
		})
		mm.Set(bridgev2.ChatMember{
			EventSender: ic.makeEventSender(newNick),
			Membership:  event.MembershipJoin,
			PowerLevel:  ptr.Ptr(ch.PowerLevel),
			MemberEventExtra: map[string]any{
				"reason": fmt.Sprintf("Changed nick from %s", prevNick),
			},
		})
		ic.UserLogin.QueueRemoteEvent(&simplevent.ChatInfoChange{
			EventMeta: simplevent.EventMeta{
				Type: bridgev2.RemoteEventChatInfoChange,
				LogContext: func(c zerolog.Context) zerolog.Context {
					return c.Str("source", msg.Source).Str("action", "nick change")
				},
				PortalKey: ic.makePortalKey(ch.Name),
				Timestamp: getTimeTag(msg),
			},
			ChatInfoChange: &bridgev2.ChatInfoChange{
				MemberChanges: &bridgev2.ChatMemberList{
					MemberMap: mm,
				},
			},
		})
	}
}

func (ic *IRCClient) onQuit(msg ircmsg.Message) {
	ic.chatInfoCacheLock.Lock()
	defer ic.chatInfoCacheLock.Unlock()
	nick := msg.Nick()
	reason := msg.Params[0]
	if !strings.HasPrefix(strings.ToLower(reason), "quit") {
		reason = "Quit: " + reason
	}
	for ch := range ic.unlockedFindChannelsOfMember(nick) {
		delete(ch.Meta.Members, nick)
		ic.UserLogin.QueueRemoteEvent(&simplevent.ChatInfoChange{
			EventMeta: simplevent.EventMeta{
				Type: bridgev2.RemoteEventChatInfoChange,
				LogContext: func(c zerolog.Context) zerolog.Context {
					return c.Str("source", msg.Source).Str("action", "quit")
				},
				PortalKey: ic.makePortalKey(ch.Name),
				Timestamp: getTimeTag(msg),
				Sender:    ic.makeEventSender(nick),
			},
			ChatInfoChange: &bridgev2.ChatInfoChange{
				MemberChanges: &bridgev2.ChatMemberList{
					MemberMap: bridgev2.ChatMemberMap{}.Set(bridgev2.ChatMember{
						EventSender: ic.makeEventSender(nick),
						Membership:  event.MembershipLeave,
						PowerLevel:  ptr.Ptr(0),
						MemberEventExtra: map[string]any{
							"reason": reason,
						},
						PrevMembership: event.MembershipJoin,
					}),
				},
			},
		})
	}
}

func (ic *IRCClient) onJoinPart(msg ircmsg.Message) {
	if msg.Command == "JOIN" && msg.Nick() == ic.Conn.CurrentNick() {
		// Let the names handler deal with self-joins
		return
	}
	ic.chatInfoCacheLock.Lock()
	info, ok := ic.chatInfoCache[msg.Params[0]]
	if ok {
		if msg.Command == "JOIN" {
			info.Members[msg.Nick()] = 0
		} else {
			delete(info.Members, msg.Nick())
		}
	}
	ic.chatInfoCacheLock.Unlock()
	member := bridgev2.ChatMember{
		EventSender:    ic.makeEventSender(msg.Nick()),
		Membership:     event.MembershipLeave,
		PowerLevel:     ptr.Ptr(0),
		PrevMembership: event.MembershipJoin,
	}
	if msg.Command == "JOIN" {
		member.Membership = event.MembershipJoin
		member.PrevMembership = ""
	}
	ic.UserLogin.QueueRemoteEvent(&simplevent.ChatInfoChange{
		EventMeta: simplevent.EventMeta{
			Type: bridgev2.RemoteEventChatInfoChange,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Str("source", msg.Source)
			},
			PortalKey: ic.makePortalKey(msg.Params[0]),
			Sender:    ic.makeEventSender(msg.Nick()),
			Timestamp: getTimeTag(msg),
		},
		ChatInfoChange: &bridgev2.ChatInfoChange{
			MemberChanges: &bridgev2.ChatMemberList{
				MemberMap: bridgev2.ChatMemberMap{}.Set(member),
			},
		},
	})
}

func (ic *IRCClient) onMode(msg ircmsg.Message) {
	if ic.isDM(msg.Params[0]) {
		return
	}
	// TODO
}

func getTimeTag(msg ircmsg.Message) time.Time {
	ok, timeTag := msg.GetTag("time")
	var ts time.Time
	if ok {
		ts, _ = time.Parse("2006-01-02T15:04:05.999Z", timeTag)
	}
	return ts
}

func (ic *IRCClient) onMessage(msg ircmsg.Message) {
	senderNick := msg.Nick()
	targetChannel := msg.Params[0]
	if senderNick == "" || strings.ContainsRune(senderNick, '.') || targetChannel == "*" {
		// TODO maybe do something with these
		return
	} else if senderNick == ic.Conn.CurrentNick() {
		ic.sendWaitersLock.Lock()
		waiter, ok := ic.sendWaiters[targetChannel]
		if ok && msg.Command != waiter.cmd {
			ok = false
		}
		if ok {
			delete(ic.sendWaiters, targetChannel)
		}
		ic.sendWaitersLock.Unlock()
		if ok {
			waiter.ch <- &msg
			return
		}
	} else if ic.isDM(targetChannel) {
		targetChannel = senderNick
	}
	ic.UserLogin.Log.Trace().Any("evt", msg).Msg("Received message")
	meta := simplevent.EventMeta{
		Type: bridgev2.RemoteEventMessage,
		LogContext: func(c zerolog.Context) zerolog.Context {
			return c.
				Str("source", msg.Source).
				Str("channel", targetChannel)
		},
		PortalKey:    ic.makePortalKey(targetChannel),
		Sender:       ic.makeEventSender(senderNick),
		CreatePortal: true,
		Timestamp:    getTimeTag(msg),
	}
	switch msg.Command {
	case "REDACT":
		// TODO reaction redactions
		meta.Type = bridgev2.RemoteEventMessageRemove
		ic.UserLogin.QueueRemoteEvent(&simplevent.MessageRemove{
			EventMeta:     meta,
			TargetMessage: makeProperMessageID(ic.NetMeta.Name, msg.Params[1]),
			//Reason:        msg.Params[2],
		})
	case "TAGMSG":
		_, msgID := msg.GetTag("msgid")
		_, reply := msg.GetTag("+draft/reply")
		_, reaction := msg.GetTag("+draft/react")
		_, typing := msg.GetTag("+typing")
		if typing != "" {
			meta.Type = bridgev2.RemoteEventTyping
			var timeout time.Duration
			switch typing {
			case "active":
				timeout = 6 * time.Second
			}
			ic.UserLogin.QueueRemoteEvent(&simplevent.Typing{
				EventMeta: meta,
				Timeout:   timeout,
				Type:      bridgev2.TypingTypeText,
			})
		} else if reply != "" && reaction != "" {
			meta.Type = bridgev2.RemoteEventReaction
			ic.UserLogin.QueueRemoteEvent(&simplevent.Reaction{
				EventMeta:     meta,
				TargetMessage: makeProperMessageID(ic.NetMeta.Name, reply),
				EmojiID:       networkid.EmojiID(variationselector.Remove(reaction)),
				Emoji:         reaction,
				ReactionDBMeta: &ReactionMetadata{
					MessageID: msgID,
				},
			})
		}
	default:
		ic.UserLogin.QueueRemoteEvent(&simplevent.Message[*ircmsg.Message]{
			EventMeta:          meta,
			Data:               &msg,
			ID:                 makeMessageID(ic.NetMeta.Name, &msg),
			ConvertMessageFunc: ic.convertMessage,
		})
	}
}

func (ic *IRCClient) convertMessage(
	ctx context.Context,
	portal *bridgev2.Portal,
	intent bridgev2.MatrixAPI,
	data *ircmsg.Message,
) (*bridgev2.ConvertedMessage, error) {
	content := ircfmt.ASCIIToContent(data.Params[1])
	if data.Command == "NOTICE" {
		content.MsgType = event.MsgNotice
	} else if data.Command == "CTCP_ACTION" {
		content.MsgType = event.MsgEmote
	}
	ok, replyToID := data.GetTag("+draft/reply")
	var replyTo *networkid.MessageOptionalPartID
	if ok {
		replyTo = &networkid.MessageOptionalPartID{
			MessageID: makeProperMessageID(ic.NetMeta.Name, replyToID),
			PartID:    ptr.Ptr(networkid.PartID("")),
		}
	}
	return &bridgev2.ConvertedMessage{
		ReplyTo: replyTo,
		Parts: []*bridgev2.ConvertedMessagePart{{
			Type:    event.EventMessage,
			Content: content,
		}},
	}, nil
}

func (ic *IRCClient) onUsers(message ircmsg.Message) {
	if message.Params[1] != "@" && message.Params[1] != "=" && message.Params[1] != "*" {
		ic.UserLogin.Log.Debug().
			Str("channel", message.Params[2]).
			Msg("Ignoring users list")
		return
	}
	ic.UserLogin.Log.Debug().
		Str("channel", message.Params[2]).
		Msg("Received users list")
	ic.chatInfoCacheLock.Lock()
	defer ic.chatInfoCacheLock.Unlock()
	info := ic.unlockedGetOrCreateChatInfo(message.Params[2])
	if info.MembersComplete {
		clear(info.Members)
	}
	for nick := range strings.FieldsSeq(message.Params[3]) {
		pl, ok := ic.isupport.PLPrefixes[nick[0]]
		if ok {
			nick = nick[1:]
		}
		info.Members[nick] = pl
	}
}

func (ic *IRCClient) onUsersEnd(message ircmsg.Message) {
	ic.chatInfoCacheLock.Lock()
	defer ic.chatInfoCacheLock.Unlock()
	info, ok := ic.chatInfoCache[message.Params[1]]
	if !ok {
		return
	}
	info.MembersComplete = true
	ic.UserLogin.QueueRemoteEvent(&simplevent.ChatResync{
		EventMeta: simplevent.EventMeta{
			Type: bridgev2.RemoteEventChatResync,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Str("source", message.Source).Str("action", "users end resync")
			},
			PortalKey:    ic.makePortalKey(message.Params[1]),
			CreatePortal: true,
			Timestamp:    getTimeTag(message),
		},
	})
}

func (ic *IRCClient) onTopic(message ircmsg.Message) {
	ic.chatInfoCacheLock.Lock()
	defer ic.chatInfoCacheLock.Unlock()
	ic.unlockedGetOrCreateChatInfo(message.Params[1]).Topic = message.Params[2]
}

func (ic *IRCClient) onTopicTime(message ircmsg.Message) {
	ic.chatInfoCacheLock.Lock()
	defer ic.chatInfoCacheLock.Unlock()
	chatInfo := ic.unlockedGetOrCreateChatInfo(message.Params[1])
	setAtInt, _ := strconv.ParseInt(message.Params[3], 10, 64)
	if setAtInt > 0 {
		chatInfo.TopicSetAt = time.Unix(setAtInt, 0)
	}
	nuh, _ := ircmsg.ParseNUH(message.Params[2])
	chatInfo.TopicSetBy = ic.makeUserID(nuh.Name)
}
