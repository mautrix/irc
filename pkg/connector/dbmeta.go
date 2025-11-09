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
	"maunium.net/go/mautrix/bridgev2/database"
)

func (ic *IRCConnector) GetDBMetaTypes() database.MetaTypes {
	return database.MetaTypes{
		Portal:  nil,
		Ghost:   nil,
		Message: nil,
		Reaction: func() any {
			return &ReactionMetadata{}
		},
		UserLogin: func() any {
			return &UserLoginMetadata{}
		},
	}
}

type UserLoginMetadata struct {
	Server   string   `json:"server"`
	Nick     string   `json:"nick"`
	RealName string   `json:"real_name"`
	Password string   `json:"password"`
	SASLUser string   `json:"sasl_user"`
	Channels []string `json:"channels"`
}

type ReactionMetadata struct {
	MessageID string `json:"message_id"`
}
