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
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/ergochat/irc-go/ircmsg"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"
)

func makeUserLoginID(netName string, userMXID id.UserID) networkid.UserLoginID {
	return networkid.UserLoginID(fmt.Sprintf("%s:%s", netName, userMXID))
}

func makeProperMessageID(netName string, id string) networkid.MessageID {
	return networkid.MessageID(fmt.Sprintf("%s:id:%s", netName, id))
}

func makeMessageID(netName string, msg *ircmsg.Message) networkid.MessageID {
	ok, msgID := msg.GetTag("msgid")
	if ok {
		return makeProperMessageID(netName, msgID)
	}
	ok, ts := msg.GetTag("time")
	if ok {
		return networkid.MessageID(fmt.Sprintf("%s:time:%s:%s:%s", netName, msg.Params[0], msg.Source, ts))
	}
	hash := sha256.Sum256([]byte(msg.Params[1]))
	approxTime := int(float64(time.Now().Unix()) / 60)
	return networkid.MessageID(fmt.Sprintf("%s:hash:%s:%s:%d:%x", netName, msg.Params[0], msg.Source, approxTime, hash[:16]))
}

func parseProperMessageID(msgID networkid.MessageID) (netName, realID string) {
	parts := strings.SplitN(string(msgID), ":", 3)
	netName = parts[0]
	if len(parts) == 3 && parts[1] == "id" {
		realID = parts[2]
	}
	return
}

func (ic *IRCClient) makePortalID(channel string) networkid.PortalID {
	mappedChannel := ic.isupport.CaseMapping(channel)
	ic.casemappedNames.Set(mappedChannel, channel)
	return networkid.PortalID(fmt.Sprintf("%s:%s", ic.NetMeta.Name, mappedChannel))
}

func parsePortalID(portalID networkid.PortalID) (netName, channel string, err error) {
	parts := strings.SplitN(string(portalID), ":", 2)
	if len(parts) != 2 {
		err = fmt.Errorf("invalid portal ID: %s", portalID)
		return
	}
	return parts[0], parts[1], nil
}

func (ic *IRCClient) isDM(channel string) bool {
	return !strings.ContainsRune(ic.isupport.ChanTypes, rune(channel[0]))
}

func (ic *IRCClient) makePortalKey(channel string) (key networkid.PortalKey) {
	key.ID = ic.makePortalID(channel)
	if ic.Main.Bridge.Config.SplitPortals || ic.isDM(channel) {
		key.Receiver = ic.UserLogin.ID
	}
	return
}

func (ic *IRCClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	netName, nick, err := ic.parseUserID(userID)
	return err == nil && netName == ic.NetMeta.Name && nick == ic.isupport.CaseMapping(ic.Conn.CurrentNick())
}

func (ic *IRCClient) parseUserID(userID networkid.UserID) (netName, nick string, err error) {
	parts := strings.SplitN(string(userID), "_", 2)
	if len(parts) != 2 {
		err = fmt.Errorf("invalid user ID format: %s", userID)
		return
	}
	return parts[0], parts[1], nil
}

func (ic *IRCClient) makeUserID(nick string) networkid.UserID {
	if nick == "" {
		return ""
	}
	mappedNick := ic.isupport.CaseMapping(nick)
	ic.casemappedNames.Set(mappedNick, nick)
	return networkid.UserID(fmt.Sprintf("%s_%s", ic.NetMeta.Name, mappedNick))
}

func (ic *IRCClient) makeEventSender(nick string) bridgev2.EventSender {
	return bridgev2.EventSender{
		IsFromMe: nick == ic.Conn.CurrentNick(),
		Sender:   ic.makeUserID(nick),
	}
}
