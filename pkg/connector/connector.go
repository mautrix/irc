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
	"sync"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/commands"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-irc/pkg/connector/ircdb"
)

type netNickPair struct {
	net  string
	nick string
}

type IRCConnector struct {
	Bridge *bridgev2.Bridge
	Config Config
	DB     *ircdb.IRCDB

	userLogins     map[netNickPair]*IRCClient
	userLoginsLock sync.RWMutex
}

var _ bridgev2.NetworkConnector = (*IRCConnector)(nil)

func (ic *IRCConnector) Init(bridge *bridgev2.Bridge) {
	ic.userLogins = make(map[netNickPair]*IRCClient)
	ic.Bridge = bridge
	ic.DB = ircdb.New(bridge.DB.Database, bridge.Log.With().Str("db_section", "irc").Logger())
	bridge.Commands.(*commands.Processor).AddHandlers(cmdJoin)
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

func (ic *IRCConnector) removeLoginFromMap(cli *IRCClient) {
	nick := cli.Conn.CurrentNick()
	if nick == "" {
		nick = cli.Conn.Nick
	}
	ic.addLoginToMap(cli, nick, "")
}

func (ic *IRCConnector) addLoginToMap(cli *IRCClient, oldNick, newNick string) {
	newNick = cli.isupport.CaseMapping(newNick)
	oldNick = cli.isupport.CaseMapping(oldNick)
	ic.userLoginsLock.Lock()
	defer ic.userLoginsLock.Unlock()
	if oldNick != "" && ic.userLogins[netNickPair{net: cli.NetMeta.Name, nick: oldNick}] == cli {
		delete(ic.userLogins, netNickPair{net: cli.NetMeta.Name, nick: oldNick})
	}
	if newNick != "" {
		ic.userLogins[netNickPair{net: cli.NetMeta.Name, nick: newNick}] = cli
	}
}

func (ic *IRCConnector) getLoginByNick(net, nick string) networkid.UserLoginID {
	ic.userLoginsLock.RLock()
	defer ic.userLoginsLock.RUnlock()
	cli, ok := ic.userLogins[netNickPair{net: net, nick: nick}]
	if ok && cli.Conn != nil && cli.isupport.CaseMapping(cli.Conn.CurrentNick()) == nick {
		return cli.UserLogin.ID
	}
	return ""
}
