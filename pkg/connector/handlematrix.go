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
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/event"
)

type sendWaiter struct {
	ch chan *ircmsg.Message
}

var (
	ErrNoPublicMedia = bridgev2.WrapErrorInStatus(errors.New("matrix connector doesn't support public media")).WithIsCertain(true).WithErrorAsMessage().WithErrorReason(event.MessageStatusUnsupported)
)

func (ic *IRCClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (message *bridgev2.MatrixMessageResponse, err error) {
	netName, channel, err := parsePortalID(msg.Portal.ID)
	if err != nil {
		return nil, err
	} else if netName != ic.NetMeta.Name {
		return nil, fmt.Errorf("%w (netname mismatch %q != %q)", bridgev2.ErrResolveIdentifierTryNext, netName, ic.NetMeta.Name)
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
	if msg.Content.MsgType == event.MsgNotice {
		cmd = "NOTICE"
	} else if msg.Content.MsgType == event.MsgEmote {
		body = fmt.Sprintf("\x01ACTION %s\x01", body)
	}
	tags := make(map[string]string)
	if msg.ReplyTo != nil {
		_, msgID := parseProperMessageID(msg.ReplyTo.ID)
		if msgID != "" {
			tags["+draft/reply"] = msgID
		}
	}
	wrapped := ircmsg.MakeMessage(tags, "", cmd, channel, body)
	_, willEcho := ic.Conn.AcknowledgedCaps()["echo-message"]
	ch := make(chan *ircmsg.Message, 1)
	if willEcho {
		ic.sendWaitersLock.Lock()
		ic.sendWaiters[channel] = sendWaiter{ch: ch}
		ic.sendWaitersLock.Unlock()
	} else {
		ch <- &wrapped
	}
	err = ic.Conn.SendIRCMessage(wrapped)
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		return &bridgev2.MatrixMessageResponse{
			DB: &database.Message{
				ID:       makeMessageID(ic.NetMeta.Name, resp),
				SenderID: ic.makeUserID(ic.Conn.CurrentNick()),
			},
		}, nil
	}
}
