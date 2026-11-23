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
	"go.mau.fi/util/l10n"
)

var L = l10n.NewNamespace("mautrix-irc")

var (
	msgFlowNickName                = L.Define("login.flow.nick.name", "Nick")
	msgFlowNickDescription         = L.Define("login.flow.nick.description", "Connect to an IRC network with the address, nick and optionally password")
	msgFieldNetworkName            = L.Define("login.field.network.name", "IRC network")
	msgFieldNickName               = L.Define("login.field.nick.name", "IRC nick")
	msgFieldCredentialsName        = L.Define("login.field.credentials.name", "Auth credentials")
	msgFieldCredentialsDescription = L.Define("login.field.credentials.description", "SASL username and password separated by a colon. To disable authentication, enter just a single colon (`:`)")
	msgStepCompleteInstructions    = L.Define(
		"login.step.complete.instructions", "Connecting to {network} as {nick}",
		l10n.WithComment("network is the display name of the IRC network, nick is the IRC nickname"),
	)
)
