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
	"maps"
	"slices"
	"strings"

	"github.com/pkg/errors"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/status"
)

var loginFlows = []bridgev2.LoginFlow{{
	Name:        "Nick",
	Description: "Connect to an IRC network with the address, nick and optionally password",
	ID:          "nick",
}}

func (ic *IRCConnector) GetLoginFlows() []bridgev2.LoginFlow {
	return loginFlows
}

func (ic *IRCConnector) CreateLogin(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
	if flowID != loginFlows[0].ID {
		return nil, fmt.Errorf("unknown flow ID: %s", flowID)
	}
	return &IRCLogin{User: user, Main: ic}, nil
}

type IRCLogin struct {
	User *bridgev2.User
	Main *IRCConnector
}

var _ bridgev2.LoginProcessUserInput = (*IRCLogin)(nil)

func (zl *IRCLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	// Pre-allocate an ident user
	_, err := zl.Main.DB.GetIdent(ctx, zl.User.MXID)
	if err != nil {
		return nil, err
	}
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeUserInput,
		StepID:       "fi.mau.irc.credentials",
		Instructions: "",
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: []bridgev2.LoginInputDataField{{
				Type:    bridgev2.LoginInputFieldTypeSelect,
				ID:      "network",
				Name:    "IRC network",
				Options: slices.Collect(maps.Keys(zl.Main.Config.Networks)),
			}, {
				Type: bridgev2.LoginInputFieldTypeUsername,
				ID:   "nick",
				Name: "IRC nick",
			}, {
				Type:        bridgev2.LoginInputFieldTypePassword,
				ID:          "credentials",
				Name:        "Auth credentials",
				Description: "SASL username and password separated by a colon. To disable authentication, enter just a single colon (`:`)",
				Pattern:     "^.*:.*$",
			}},
		},
	}, nil
}

func (zl *IRCLogin) Cancel() {}

var (
	ErrUnknownNetwork = bridgev2.WrapRespErr(errors.New("unknown network"), mautrix.MNotFound)
)

func (zl *IRCLogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	netName := strings.ToLower(input["network"])
	netMeta, ok := zl.Main.Config.Networks[netName]
	if !ok {
		return nil, ErrUnknownNetwork.AppendMessage(" %s", netName)
	}
	creds := strings.SplitN(strings.TrimSpace(input["credentials"]), ":", 2)
	var authUser, authPass string
	if len(creds) == 2 && creds[0] != "" && creds[1] != "" {
		authUser = creds[0]
		authPass = creds[1]
	}
	meta := &UserLoginMetadata{
		Server:   netName,
		Nick:     input["nick"],
		RealName: zl.User.MXID.String(),
		Password: authPass,
		SASLUser: authUser,
		Channels: nil,
	}
	login, err := zl.User.NewLogin(ctx, &database.UserLogin{
		ID:         makeUserLoginID(netName, zl.User.MXID),
		RemoteName: fmt.Sprintf("%s on %s", meta.Nick, netMeta.DisplayName),
		RemoteProfile: status.RemoteProfile{
			Name: meta.Nick,
		},
		Metadata: meta,
	}, &bridgev2.NewLoginParams{})
	if err != nil {
		return nil, err
	}
	go login.Client.Connect(login.Log.WithContext(zl.Main.Bridge.BackgroundCtx))
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeComplete,
		StepID:       "fi.mau.irc.complete",
		Instructions: fmt.Sprintf("Connecting to %s as %s", netMeta.DisplayName, meta.Nick),
		CompleteParams: &bridgev2.LoginCompleteParams{
			UserLoginID: login.ID,
			UserLogin:   login,
		},
	}, nil
}
