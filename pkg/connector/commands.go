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
	"bytes"
	"encoding/json"
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

var cmdSetSASL = &commands.FullHandler{
	Func: func(ce *commands.Event) {
		if len(ce.Args) == 0 {
			ce.Reply("Usage: $cmdprefix set-sasl <network> <new username>:<new password>")
			return
		}
		login := ce.Bridge.GetCachedUserLoginByID(makeUserLoginID(ce.Args[0], ce.User.MXID))
		if login == nil {
			ce.Reply("You are not logged into %s (active logins: %s)", format.SafeMarkdownCode(ce.Args[0]), getLogins(ce.User))
			return
		}
		remainingArgs := strings.TrimSpace(strings.TrimPrefix(ce.RawArgs, ce.Args[0]))
		meta := login.Metadata.(*UserLoginMetadata)
		if !strings.ContainsRune(remainingArgs, ':') {
			ce.Reply("Current SASL credentials: %s:%s", format.SafeMarkdownCode(meta.SASLUser), format.SafeMarkdownCode(meta.Password))
		} else {
			parts := strings.SplitN(remainingArgs, ":", 1)
			meta.SASLUser = parts[0]
			meta.Password = parts[1]
			err := login.Save(ce.Ctx)
			if err != nil {
				ce.Log.Err(err).Msg("Failed to save login after changing SASL credentials")
				ce.Reply("Failed to save new SASL credentials")
				return
			}
			ce.Reply("Changed SASL credentials to %s:%s", format.SafeMarkdownCode(meta.SASLUser), format.SafeMarkdownCode(meta.Password))
		}
	},
	Name:    "set-sasl",
	Aliases: []string{"sasl"},
	Help: commands.HelpMeta{
		Section:     commands.HelpSectionAuth,
		Description: "Change your SASL credentials",
		Args:        "<network> <new username> <new password>",
	},
	RequiresLogin: true,
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
			netName, _, err = parsePortalID(ce.Portal.ID)
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

var cmdRaw = &commands.FullHandler{
	Func: func(ce *commands.Event) {
		if len(ce.Args) == 0 {
			ce.Reply("Usage: $cmdprefix raw [network] [tags json] <command> [params...]")
			return
		}
		var err error
		var netName string
		if ce.Bridge.Network.(*IRCConnector).networkExists(ce.Args[0]) {
			netName = ce.Args[0]
			ce.Args = ce.Args[1:]
			ce.RawArgs = strings.TrimSpace(strings.TrimPrefix(ce.RawArgs, netName))
		} else if ce.Portal != nil {
			netName, _, err = parsePortalID(ce.Portal.ID)
			if err != nil {
				ce.Reply("Failed to parse portal ID: %s", err)
				return
			}
		} else {
			ce.Reply("The network argument is required when outside of a network room.")
			return
		}
		login := ce.Bridge.GetCachedUserLoginByID(makeUserLoginID(netName, ce.User.MXID))
		if login == nil {
			ce.Reply("You are not logged into %s (active logins: %s)", format.SafeMarkdownCode(netName), getLogins(ce.User))
			return
		}
		var tags map[string]string
		if strings.HasPrefix(ce.Args[0], "{") {
			buf := bytes.NewBufferString(ce.RawArgs)
			err = json.NewDecoder(buf).Decode(&tags)
			if err != nil {
				ce.Reply("Failed to parse tags JSON: %s", err)
				return
			}
			ce.Args = strings.Fields(strings.TrimSpace(buf.String()))
		}
		cmd := strings.ToUpper(ce.Args[0])
		args := ce.Args[1:]
		for i, arg := range args {
			if strings.HasPrefix(arg, ":") {
				args[i] = strings.Join(args[i:], " ")
				args[i] = strings.TrimPrefix(args[i], ":")
				args = args[:i+1]
				break
			}
		}

		resp, err := login.Client.(*IRCClient).SendRequest(ce.Ctx, tags, "", cmd, args...)
		if err != nil {
			ce.Reply("Failed to send command: %s", err)
		} else {
			line, _ := resp.Line()
			ce.Reply("Got response: %s", format.SafeMarkdownCode(line))
		}
	},
	Name: "raw",
	Help: commands.HelpMeta{
		Section:     commands.HelpSectionAdmin,
		Description: "Send a raw IRC command",
		Args:        "[network] [tags json] <command> [params...]",
	},
	RequiresLogin: true,
}
