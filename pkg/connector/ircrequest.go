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
	"time"

	"github.com/ergochat/irc-go/ircevent"
	"github.com/ergochat/irc-go/ircmsg"
	"github.com/pkg/errors"
)

func (ic *IRCClient) onDisconnect(message ircmsg.Message) {
	ic.sendWaitersLock.Lock()
	defer ic.sendWaitersLock.Unlock()
	for _, waiter := range ic.sendWaiters {
		close(waiter.ch)
	}
	clear(ic.sendWaiters)
}

func (ic *IRCClient) onPotentialEchoMessage(msg ircmsg.Message) bool {
	if msg.Nick() != ic.Conn.CurrentNick() {
		return false
	}
	ic.sendWaitersLock.Lock()
	waiter, ok := ic.sendWaiters[msg.Params[0]]
	ok = ok && msg.Command == waiter.cmd
	if ok {
		delete(ic.sendWaiters, msg.Params[0])
	}
	ic.sendWaitersLock.Unlock()
	if ok {
		waiter.ch <- &msg
		return true
	}
	return false
}

type IRCError struct {
	Msg *ircmsg.Message
}

func (ie *IRCError) Error() string {
	return fmt.Sprintf("%s: %s", ie.Msg.Command, ie.Msg.Params[len(ie.Msg.Params)-1])
}

func (ic *IRCClient) SendRequest(ctx context.Context, tags map[string]string, waiterCmd, cmd string, args ...string) (*ircmsg.Message, error) {
	channel := args[0]
	labelResp, err := ic.Conn.GetLabeledResponse(tags, cmd, args...)
	if err != nil && !errors.Is(err, ircevent.CapabilityNotNegotiated) {
		return nil, err
	} else if labelResp != nil {
		waiterCmd = cmd
		if cmd == "RELAYMSG" {
			waiterCmd = "PRIVMSG"
		}
		if labelResp.Command == "BATCH" {
			switch labelResp.Params[1] {
			case "draft/multiline":
				return multilineBatchToMessage(labelResp), nil
			case "labeled-response":
				firstPart := labelResp.Items[0]
				// This is a hack for service DM responses among other things
				// First item in the batch is the echo message, the rest are the response from the bot
				if firstPart.Command == waiterCmd && firstPart.Nick() == ic.Conn.CurrentNick() &&
					len(labelResp.Items) > 1 && labelResp.Items[1].Nick() != ic.Conn.CurrentNick() {
					labelResp.Items = labelResp.Items[1:]
					labelResp.Source = labelResp.Items[0].Source
					ic.onMessage(*multilineBatchToMessage(labelResp))
					return &firstPart.Message, nil
				}
			}
		}
		if labelResp.Command != waiterCmd {
			return nil, &IRCError{Msg: &labelResp.Message}
		}
		return &labelResp.Message, nil
	}
	if waiterCmd == "" {
		waiterCmd = cmd
	}
	wrapped := ircmsg.MakeMessage(tags, "", cmd, args...)
	_, willEcho := ic.Conn.AcknowledgedCaps()["echo-message"]
	if cmd == "PRIVMSG" && (args[0] == "nickserv" || args[0] == "chanserv") {
		// Some servers like libera are buggy and don't echo messages sent to services
		willEcho = false
	}
	ch := make(chan *ircmsg.Message, 1)
	var timeoutCh <-chan time.Time
	if willEcho {
		ic.sendWaitersLock.Lock()
		ic.sendWaiters[channel] = sendWaiter{ch: ch, cmd: waiterCmd}
		ic.sendWaitersLock.Unlock()
		timeoutCh = time.After(15 * time.Second)
	} else {
		timeoutCh = time.After(1 * time.Second)
	}
	err = ic.Conn.SendIRCMessage(wrapped)
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		if resp == nil {
			return nil, fmt.Errorf("no echo received")
		} else if resp.Command != waiterCmd {
			return nil, &IRCError{Msg: resp}
		}
		return resp, nil
	case <-timeoutCh:
		if willEcho {
			return nil, fmt.Errorf("timeout waiting for echo message")
		}
		// We're not waiting for an echo, which means the timeout is a success
		return &wrapped, nil
	}
}

func (ic *IRCClient) onFallbackReply(message ircmsg.Message) bool {
	if len(message.Params) == 0 {
		return false
	}
	ic.sendWaitersLock.Lock()
	defer ic.sendWaitersLock.Unlock()
	isError := message.Params[0] == ic.Conn.CurrentNick() &&
		len(message.Command) == 3 &&
		(message.Command[0] == '4' || message.Command[0] == '5' || isNon45Error(message.Command))
	channel := message.Params[min(1, len(message.Params)-1)]
	waiter, ok := ic.sendWaiters[channel]
	if !ok {
		if !isError && len(message.Params) > 1 {
			channel = message.Params[0]
			waiter, ok = ic.sendWaiters[channel]
		}
		if !ok {
			return false
		}
	}
	if message.Command == waiter.cmd {
		if message.Nick() != ic.Conn.CurrentNick() {
			return false
		}
	} else if !isError {
		return false
	}
	delete(ic.sendWaiters, channel)
	waiter.ch <- &message
	return true
}

func isNon45Error(cmd string) bool {
	switch cmd {
	case ircevent.ERR_INVALIDMODEPARAM, ircevent.ERR_LISTMODEALREADYSET, ircevent.ERR_LISTMODENOTSET,
		ircevent.ERR_NOPRIVS, ircevent.ERR_NICKLOCKED:
		return true
	default:
		return false
	}
}
