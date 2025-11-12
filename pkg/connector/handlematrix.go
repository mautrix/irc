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
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/ergochat/irc-go/ircmsg"
	"go.mau.fi/util/exslices"
	"go.mau.fi/util/variationselector"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"

	"go.mau.fi/mautrix-irc/pkg/ircfmt"
)

var (
	_ bridgev2.ReactionHandlingNetworkAPI   = (*IRCClient)(nil)
	_ bridgev2.RedactionHandlingNetworkAPI  = (*IRCClient)(nil)
	_ bridgev2.TypingHandlingNetworkAPI     = (*IRCClient)(nil)
	_ bridgev2.RoomTopicHandlingNetworkAPI  = (*IRCClient)(nil)
	_ bridgev2.MembershipHandlingNetworkAPI = (*IRCClient)(nil)
)

type sendWaiter struct {
	ch  chan *ircmsg.Message
	cmd string
}

var (
	ErrNoPublicMedia = bridgev2.WrapErrorInStatus(errors.New("matrix connector doesn't support public media")).WithIsCertain(true).WithErrorAsMessage().WithErrorReason(event.MessageStatusUnsupported)
)

const specialChars = `!+%@&#$:'"?*,. `

func filterPerMessageName(nick string) string {
	return string(slices.DeleteFunc([]rune(nick), func(r rune) bool {
		return r > unicode.MaxLatin1 || strings.ContainsRune(specialChars, r) || !unicode.IsPrint(r)
	}))
}

func (ic *IRCClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (message *bridgev2.MatrixMessageResponse, err error) {
	channel, err := ic.parsePortalID(msg.Portal.ID)
	if err != nil {
		return nil, err
	}
	body := ircfmt.ContentToASCII(ctx, msg.Content)
	if msg.Content.MsgType.IsMedia() {
		pm, ok := ic.Main.Bridge.Matrix.(bridgev2.MatrixConnectorWithPublicMedia)
		if !ok {
			return nil, ErrNoPublicMedia
		}
		url, err := pm.GetPublicMediaAddressForEvent(ctx, msg.Content)
		if err != nil {
			return nil, err
		}
		if body != "" {
			body += " "
		}
		body += fmt.Sprintf("<%s>", url)
	}
	cmd := "PRIVMSG"
	var waiterCmd string
	if msg.Content.MsgType == event.MsgNotice {
		cmd = "NOTICE"
	} else if msg.Content.MsgType == event.MsgEmote {
		body = fmt.Sprintf("\x01ACTION %s\x01", body)
		waiterCmd = "CTCP_ACTION"
	}
	tags := make(map[string]string)
	_, canTag := ic.Conn.AcknowledgedCaps()["message-tags"]
	if msg.ReplyTo != nil && canTag {
		_, msgID := parseProperMessageID(msg.ReplyTo.ID)
		if msgID != "" {
			tags["+draft/reply"] = msgID
		}
	}
	var resp *ircmsg.Message
	relayChar, canRelay := ic.Conn.AcknowledgedCaps()["draft/relaymsg"]
	if canRelay && (msg.OrigSender != nil || msg.Content.BeeperPerMessageProfile != nil) {
		var overrideNick string
		if msg.OrigSender != nil {
			overrideNick = filterPerMessageName(msg.OrigSender.FormattedName)
			if overrideNick == "" {
				overrideNick = filterPerMessageName(msg.OrigSender.UserID.Localpart())
			}
		} else {
			overrideNick = filterPerMessageName(msg.Content.BeeperPerMessageProfile.Displayname)
		}
		if overrideNick == "" {
			return nil, fmt.Errorf("invalid relay nick")
		}
		if relayChar != "" {
			relayChar = "m" + relayChar
		}
		resp, err = ic.SendRequest(ctx, tags, "", "RELAYMSG", channel, relayChar+overrideNick, body)
	} else {
		resp, err = ic.SendRequest(ctx, tags, waiterCmd, cmd, channel, body)
	}
	if err != nil {
		return nil, err
	}
	return &bridgev2.MatrixMessageResponse{
		DB: &database.Message{
			ID:        makeMessageID(ic.NetMeta.Name, resp),
			SenderID:  ic.makeUserID(ic.Conn.CurrentNick()),
			Timestamp: getTimeTag(*resp),
		},
	}, nil
}

func (ic *IRCClient) PreHandleMatrixReaction(ctx context.Context, msg *bridgev2.MatrixReaction) (bridgev2.MatrixReactionPreResponse, error) {
	_, canTag := ic.Conn.AcknowledgedCaps()["message-tags"]
	if !canTag {
		return bridgev2.MatrixReactionPreResponse{}, fmt.Errorf("server does not support message-tags")
	}
	_, err := ic.parsePortalID(msg.Portal.ID)
	if err != nil {
		return bridgev2.MatrixReactionPreResponse{}, err
	}
	_, msgID := parseProperMessageID(msg.TargetMessage.ID)
	if msgID == "" {
		return bridgev2.MatrixReactionPreResponse{}, fmt.Errorf("message doesn't have a proper ID")
	}
	return bridgev2.MatrixReactionPreResponse{
		SenderID:     ic.makeUserID(ic.Conn.CurrentNick()),
		EmojiID:      networkid.EmojiID(variationselector.Remove(msg.Content.RelatesTo.Key)),
		Emoji:        msg.Content.RelatesTo.Key,
		MaxReactions: 0,
	}, nil
}

func (ic *IRCClient) HandleMatrixReaction(ctx context.Context, msg *bridgev2.MatrixReaction) (reaction *database.Reaction, err error) {
	channel, err := ic.parsePortalID(msg.Portal.ID)
	if err != nil {
		return nil, err
	}
	_, msgID := parseProperMessageID(msg.TargetMessage.ID)
	if msgID == "" {
		return nil, fmt.Errorf("message doesn't have a proper ID")
	}

	resp, err := ic.SendRequest(ctx, map[string]string{
		"+draft/reply": msgID,
		"+draft/react": msg.Content.RelatesTo.Key,
	}, "", "TAGMSG", channel, "")
	if err != nil {
		return nil, err
	}
	_, reactionMsgID := resp.GetTag("msgid")
	return &database.Reaction{
		Timestamp: getTimeTag(*resp),
		Metadata: &ReactionMetadata{
			MessageID: reactionMsgID,
		},
	}, nil
}

func (ic *IRCClient) HandleMatrixReactionRemove(ctx context.Context, msg *bridgev2.MatrixReactionRemove) error {
	_, canRedact := ic.Conn.AcknowledgedCaps()["draft/message-redaction"]
	if !canRedact {
		return fmt.Errorf("server does not support draft/message-redaction")
	}
	channel, err := ic.parsePortalID(msg.Portal.ID)
	if err != nil {
		return err
	}
	msgID := msg.TargetReaction.Metadata.(*ReactionMetadata).MessageID
	if msgID == "" {
		return fmt.Errorf("no message ID stored in reaction metadata")
	}
	_, err = ic.SendRequest(ctx, nil, "", "REDACT", channel, msgID)
	if err != nil {
		return err
	}
	return nil
}

func (ic *IRCClient) HandleMatrixMessageRemove(ctx context.Context, msg *bridgev2.MatrixMessageRemove) error {
	_, canRedact := ic.Conn.AcknowledgedCaps()["draft/message-redaction"]
	if !canRedact {
		return fmt.Errorf("server does not support draft/message-redaction")
	}
	channel, err := ic.parsePortalID(msg.Portal.ID)
	if err != nil {
		return err
	}
	_, msgID := parseProperMessageID(msg.TargetMessage.ID)
	if msgID == "" {
		return fmt.Errorf("message doesn't have a proper ID")
	}
	_, err = ic.SendRequest(ctx, nil, "", "REDACT", channel, msgID, msg.Content.Reason)
	if err != nil {
		return err
	}
	return nil
}

func (ic *IRCClient) HandleMatrixTyping(ctx context.Context, msg *bridgev2.MatrixTyping) error {
	channel, err := ic.parsePortalID(msg.Portal.ID)
	if err != nil {
		return err
	}
	_, canTag := ic.Conn.AcknowledgedCaps()["message-tags"]
	if !canTag {
		return nil
	}
	typingState := "active"
	if !msg.IsTyping {
		typingState = "done"
	}
	_, err = ic.SendRequest(ctx, map[string]string{
		"+typing": typingState,
	}, "", "TAGMSG", channel)
	return err
}

func (ic *IRCClient) HandleMatrixRoomTopic(ctx context.Context, msg *bridgev2.MatrixRoomTopic) (bool, error) {
	channel, err := ic.parsePortalID(msg.Portal.ID)
	if err != nil {
		return false, err
	}
	resp, err := ic.SendRequest(ctx, nil, "", "TOPIC", channel, msg.Content.Topic)
	if err != nil {
		return false, err
	}
	msg.Portal.Topic = resp.Params[1]
	msg.Portal.TopicSet = msg.Content.Topic == resp.Params[1]
	return true, nil
}

func (ic *IRCClient) HandleMatrixMembership(ctx context.Context, msg *bridgev2.MatrixMembershipChange) (bool, error) {
	switch msg.Type {
	case bridgev2.Leave:
		channel, err := ic.parsePortalID(msg.Portal.ID)
		if err != nil {
			return false, err
		} else if ic.isDM(channel) {
			// Leaving DMs is a no-op
			return true, nil
		}
		meta := ic.UserLogin.Metadata.(*UserLoginMetadata)
		meta.Channels = exslices.FastDeleteItem(meta.Channels, channel)
		err = ic.UserLogin.Save(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to update autojoin channels: %w", err)
		}
		_, err = ic.SendRequest(ctx, nil, "", "PART", channel)
		if err != nil {
			return false, fmt.Errorf("failed to part channel: %w", err)
		}
		return true, nil
	default:
		return false, fmt.Errorf("unsupported membership change")
	}
}
