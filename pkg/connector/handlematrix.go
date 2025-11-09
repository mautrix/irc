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

	"github.com/ergochat/irc-go/ircmsg"
	"go.mau.fi/util/variationselector"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
)

var (
	_ bridgev2.ReactionHandlingNetworkAPI  = (*IRCClient)(nil)
	_ bridgev2.RedactionHandlingNetworkAPI = (*IRCClient)(nil)
)

type sendWaiter struct {
	ch  chan *ircmsg.Message
	cmd string
}

var (
	ErrNoPublicMedia = bridgev2.WrapErrorInStatus(errors.New("matrix connector doesn't support public media")).WithIsCertain(true).WithErrorAsMessage().WithErrorReason(event.MessageStatusUnsupported)
)

func (ic *IRCClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (message *bridgev2.MatrixMessageResponse, err error) {
	channel, err := ic.parsePortalID(msg.Portal.ID)
	if err != nil {
		return nil, err
	}
	body := msg.Content.Body
	if msg.Content.MsgType.IsMedia() {
		if body == msg.Content.GetFileName() {
			body = ""
		}
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
	wrapped := ircmsg.MakeMessage(tags, "", cmd, channel, body)
	resp, err := ic.sendAndWait(ctx, channel, wrapped, waiterCmd)
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

func (ic *IRCClient) sendAndWait(ctx context.Context, channel string, wrapped ircmsg.Message, waiterCmd string) (*ircmsg.Message, error) {
	if waiterCmd == "" {
		waiterCmd = wrapped.Command
	}
	_, willEcho := ic.Conn.AcknowledgedCaps()["echo-message"]
	ch := make(chan *ircmsg.Message, 1)
	if willEcho {
		ic.sendWaitersLock.Lock()
		ic.sendWaiters[channel] = sendWaiter{ch: ch, cmd: waiterCmd}
		ic.sendWaitersLock.Unlock()
	} else {
		ch <- &wrapped
	}
	err := ic.Conn.SendIRCMessage(wrapped)
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		return resp, nil
	}
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

	wrapped := ircmsg.MakeMessage(map[string]string{
		"+draft/reply": msgID,
		"+draft/react": msg.Content.RelatesTo.Key,
	}, "", "TAGMSG", channel, "")
	resp, err := ic.sendAndWait(ctx, channel, wrapped, "")
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
	wrapped := ircmsg.MakeMessage(nil, "", "REDACT", channel, msgID)
	_, err = ic.sendAndWait(ctx, channel, wrapped, "")
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
	wrapped := ircmsg.MakeMessage(nil, "", "REDACT", channel, msgID, msg.Content.Reason)
	_, err = ic.sendAndWait(ctx, channel, wrapped, "")
	if err != nil {
		return err
	}
	return nil
}
