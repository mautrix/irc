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
	"slices"
	"strings"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/commands"
	"maunium.net/go/mautrix/format"
)

func getLogins(user *bridgev2.User) string {
	logins := user.GetUserLogins()
	var loginNames []string
	for _, login := range logins {
		netName, _, _ := parseUserLoginID(login.ID)
		if login.Client.IsLoggedIn() && netName != "" {
			loginNames = append(loginNames, format.SafeMarkdownCode(netName))
		}
	}
	return strings.Join(loginNames, ", ")
}

var cmdJoin = &commands.FullHandler{
	Func: func(ce *commands.Event) {
		if len(ce.Args) == 0 {
			ce.Reply("Usage: $cmdprefix join [network] <channel>")
			return
		}
		var netName string
		if len(ce.Args) == 1 {
			if ce.Portal == nil {
				ce.Reply("The network argument is required when outside of a network room.")
				return
			}
			var err error
			_, netName, err = parsePortalID(ce.Portal.ID)
			if err != nil {
				ce.Reply("Failed to parse portal ID: %s", err)
				return
			}
		} else {
			netName = ce.Args[0]
			ce.Args = ce.Args[1:]
		}
		login := ce.Bridge.GetCachedUserLoginByID(makeUserLoginID(netName, ce.User.MXID))
		if login == nil {
			ce.Reply("You are not logged into %s (active logins: %s)", format.SafeMarkdownCode(netName), getLogins(ce.User))
			return
		}
		channel := ce.Args[0]
		meta := login.Metadata.(*UserLoginMetadata)
		cli := login.Client.(*IRCClient)
		err := cli.Conn.Join(channel)
		if err != nil {
			ce.Reply("Failed to join channel: %s", err)
			return
		}
		if slices.Contains(meta.Channels, channel) {
			ce.Reply("%s is already on your autojoin list", format.SafeMarkdownCode(channel))
		} else {
			meta.Channels = append(meta.Channels, channel)
			err = login.Save(ce.Ctx)
			if err != nil {
				ce.Log.Err(err).Msg("Failed to save login after adding autojoin channel")
			}
			ce.Reply("Joined %s and added it to your autojoin list", format.SafeMarkdownCode(channel))
		}
	},
	Name: "join",
	Help: commands.HelpMeta{
		Section:     commands.HelpSectionChats,
		Description: "Join a channel and add it to your autojoin list",
		Args:        "[network] <channel>",
	},
	RequiresLogin: true,
}
