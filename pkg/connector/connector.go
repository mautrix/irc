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

	"go.mau.fi/mautrix-irc/pkg/connector/ircdb"
)

type IRCConnector struct {
	Bridge *bridgev2.Bridge
	Config Config
	DB     *ircdb.IRCDB
}

var _ bridgev2.NetworkConnector = (*IRCConnector)(nil)

func (ic *IRCConnector) Init(bridge *bridgev2.Bridge) {
	ic.Bridge = bridge
	ic.DB = ircdb.New(bridge.DB.Database, bridge.Log.With().Str("db_section", "irc").Logger())
}

func (ic *IRCConnector) Start(ctx context.Context) error {
	err := ic.DB.Upgrade(ctx)
	if err != nil {
		return bridgev2.DBUpgradeError{Err: err, Section: "irc"}
	}
	err = ic.DB.FillCache(ctx)
	if err != nil {
		return fmt.Errorf("failed to fill ident db cache: %w", err)
	}
	return nil
}

func (ic *IRCConnector) GetName() bridgev2.BridgeName {
	return bridgev2.BridgeName{
		DisplayName:          "IRC",
		NetworkURL:           "https://ircv3.net/",
		NetworkIcon:          "mxc://maunium.net/WKXfkBwYrGLvRoOcTbeRHnqf",
		NetworkID:            "irc",
		BeeperBridgeType:     "irc",
		DefaultPort:          29343,
		DefaultCommandPrefix: "!irc",
	}
}
