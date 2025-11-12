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

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

var _ bridgev2.IdentifierValidatingNetwork = (*IRCConnector)(nil)
var _ bridgev2.IdentifierResolvingNetworkAPI = (*IRCClient)(nil)
var _ bridgev2.GhostDMCreatingNetworkAPI = (*IRCClient)(nil)

func (ic *IRCConnector) networkExists(name string) bool {
	_, netExists := ic.Config.Networks[name]
	return netExists
}

func (ic *IRCConnector) ValidateUserID(id networkid.UserID) bool {
	netName, _, err := parseUserID(id)
	if err != nil {
		return false
	}
	return ic.networkExists(netName)
}

func (ic *IRCClient) ResolveIdentifier(ctx context.Context, identifier string, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
	netName, nick, err := parseUserID(networkid.UserID(identifier))
	if err == nil && ic.Main.networkExists(netName) {
		origCase := nick
		nick = ic.isupport.CaseMapping(nick)
		err = ic.validateName(netName, nick)
		if err != nil {
			return nil, err
		}
		nick = ic.casemappedNames.GetDefault(nick, origCase)
	} else {
		nick = identifier
		if !validateIdentifier(nick) {
			return nil, fmt.Errorf("%w %s", ErrInvalidUserIDFormat, identifier)
		}
	}
	userID := ic.makeUserID(nick)
	ghost, err := ic.Main.Bridge.GetGhostByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ghost: %w", err)
	}
	return &bridgev2.ResolveIdentifierResponse{
		Ghost:    ghost,
		UserID:   ic.makeUserID(nick),
		UserInfo: ic.getUserInfo(nick),
		Chat: &bridgev2.CreateChatResponse{
			PortalKey:  ic.makePortalKey(nick),
			PortalInfo: ic.getDMInfo(nick),
		},
	}, nil
}

func (ic *IRCClient) CreateChatWithGhost(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.CreateChatResponse, error) {
	nick, err := ic.parseUserID(ghost.ID)
	if err != nil {
		return nil, err
	}
	return &bridgev2.CreateChatResponse{
		PortalKey:  ic.makePortalKey(nick),
		PortalInfo: ic.getDMInfo(nick),
	}, nil
}
