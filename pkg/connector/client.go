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
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ergochat/irc-go/ircmsg"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"go.mau.fi/util/exsync"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/status"

	"github.com/ergochat/irc-go/ircevent"
)

type IRCClient struct {
	Main      *IRCConnector
	UserLogin *bridgev2.UserLogin
	Conn      *ircevent.Connection
	NetMeta   *NetworkConfig

	stopping *exsync.Event
	stopOnce sync.Once
	stopped  *exsync.Event

	chatInfoCacheLock sync.RWMutex
	chatInfoCache     map[string]*ChatInfoCache

	isupport *ISupport

	sendWaiters     map[string]sendWaiter
	sendWaitersLock sync.Mutex

	fallbackSendLock      sync.Mutex
	fallbackExpectedReply atomic.Pointer[string]
	fallbackSendWaiter    atomic.Pointer[chan *ircmsg.Message]

	casemappedNames *exsync.Map[string, string]
}

var _ bridgev2.NetworkAPI = (*IRCClient)(nil)

func (ic *IRCConnector) LoadUserLogin(ctx context.Context, login *bridgev2.UserLogin) error {
	meta := login.Metadata.(*UserLoginMetadata)
	serverConfig, ok := ic.Config.Networks[meta.Server]
	if !ok {
		return nil
	}
	ident, err := ic.DB.GetIdent(ctx, login.UserMXID)
	if err != nil {
		return err
	}
	conn := &ircevent.Connection{
		Server:   serverConfig.Address,
		Nick:     meta.Nick,
		User:     ident,
		RealName: meta.RealName,
		RequestCaps: []string{
			"message-tags", "server-time", "echo-message", "chghost", "draft/message-redaction",
			"batch", "draft/multiline", "labeled-response",
		},
		QuitMessage: "Exiting the Matrix",
		Version:     "mautrix-irc",
		UseTLS:      serverConfig.TLS,
		EnableCTCP:  serverConfig.CTCP,
		Debug:       login.Log.GetLevel() == zerolog.TraceLevel,
		Log:         log.New(login.Log.With().Str("component", "irc").Logger(), "", 0),
	}
	if meta.SASLUser != "" {
		conn.SASLLogin = meta.SASLUser
		conn.SASLPassword = meta.Password
		conn.UseSASL = true
	}
	iclient := &IRCClient{
		Main:            ic,
		Conn:            conn,
		UserLogin:       login,
		NetMeta:         serverConfig,
		stopping:        exsync.NewEvent(),
		stopped:         exsync.NewEvent(),
		isupport:        defaultISupport,
		chatInfoCache:   make(map[string]*ChatInfoCache),
		sendWaiters:     make(map[string]sendWaiter),
		casemappedNames: exsync.NewMap[string, string](),
	}
	login.Client = iclient
	conn.AddConnectCallback(iclient.onConnect)
	conn.AddDisconnectCallback(iclient.onDisconnect)
	conn.AddBatchCallback(iclient.onBatch)
	conn.AddGlobalCallback(iclient.onFallbackReply)
	conn.AddCallback("PRIVMSG", iclient.onMessage)
	conn.AddCallback("NOTICE", iclient.onMessage)
	conn.AddCallback("TAGMSG", iclient.onMessage)
	conn.AddCallback("CTCP_ACTION", iclient.onMessage)
	conn.AddCallback("REDACT", iclient.onMessage)
	conn.AddCallback("NICK", iclient.onNick)
	conn.AddCallback("JOIN", iclient.onJoinPart)
	conn.AddCallback("PART", iclient.onJoinPart)
	conn.AddCallback("MODE", iclient.onMode)
	conn.AddCallback("QUIT", iclient.onQuit)
	conn.AddCallback(ircevent.RPL_NAMREPLY, iclient.onUsers)
	conn.AddCallback(ircevent.RPL_ENDOFNAMES, iclient.onUsersEnd)
	conn.AddCallback("TOPIC", iclient.onNewTopic)
	conn.AddCallback(ircevent.RPL_TOPIC, iclient.onOldTopic)
	conn.AddCallback(ircevent.RPL_TOPICTIME, iclient.onTopicTime)
	iclient.stopped.Set()
	return nil
}

func init() {
	status.BridgeStateHumanErrors.Update(status.BridgeStateErrorMap{
		"irc-unknown-network": "This network was removed from the bridge config",
		"irc-sasl-fail":       "Failed to authenticate on IRC",
		"irc-connect-fail":    "Failed to connect to IRC, trying to reconnect...",
		"irc-disconnected":    "Disconnected from IRC, trying to reconnect...",
	})
}

func (ic *IRCClient) Connect(ctx context.Context) {
	if ic.Conn == nil {
		ic.UserLogin.BridgeState.Send(status.BridgeState{
			StateEvent: status.StateBadCredentials,
			Error:      "irc-unknown-network",
		})
		return
	}
	ic.UserLogin.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnecting})
	go ic.connectLoop(ctx)
}

func (ic *IRCClient) connectLoop(ctx context.Context) {
	ic.stopped.Clear()
	defer ic.stopped.Set()
	connectFailures := 0
	for {
		err := ic.Conn.Connect()
		if ic.stopping.IsSet() {
			return
		}
		if errors.Is(err, ircevent.ClientHasQuit) {
			ic.UserLogin.Log.Debug().Err(err).Msg("Exiting connection loop")
			return
		} else if errors.Is(err, ircevent.SASLError) {
			ic.UserLogin.Log.Debug().Err(err).Msg("SASL failed, exiting connection loop")
			ic.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateBadCredentials,
				Error:      "irc-sasl-fail",
				Info:       map[string]any{"go_error": err.Error()},
			})
			return
		} else if err != nil {
			ic.UserLogin.Log.Err(err).Msg("Error establishing connection")
			ic.UserLogin.BridgeState.Send(status.BridgeState{
				StateEvent: status.StateTransientDisconnect,
				Error:      "irc-connect-fail",
				Info:       map[string]any{"go_error": err.Error()},
			})
			connectFailures++
		} else {
			connectFailures = 0
		}
		ic.Conn.DangerousInternalWaitForStop()
		if ic.stopping.IsSet() {
			return
		} else if connectFailures == 0 {
			err = ic.Conn.InternalGetError()
			if err != nil {
				ic.UserLogin.Log.Err(err).Msg("Error in connection")
				ic.UserLogin.BridgeState.Send(status.BridgeState{
					StateEvent: status.StateTransientDisconnect,
					Error:      "irc-disconnected",
					Info:       map[string]any{"go_error": err.Error()},
				})
				connectFailures++
			}
		}
		reconnectTime := time.Duration(2*connectFailures) * time.Second
		select {
		case <-ic.stopping.GetChan():
			return
		case <-ctx.Done():
			return
		case <-time.After(reconnectTime):
		}
	}
}

func (ic *IRCClient) Disconnect() {
	ic.stopOnce.Do(func() {
		ic.stopping.Set()
		ic.Conn.Quit()
	})
	if !ic.stopped.WaitTimeout(4 * time.Second) {
		ic.Conn.DangerousInternalKill()
	}
}

func (ic *IRCClient) IsLoggedIn() bool {
	return ic.Conn.Connected()
}

func (ic *IRCClient) LogoutRemote(ctx context.Context) {
	// There's no logout, just disconnect
}
